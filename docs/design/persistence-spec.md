# 平台持久化规范（消费流水线与存储语义）

| 项目 | 内容 |
| --- | --- |
| 状态 | Baseline（已实现，2026-08-05） |
| 关联文档 | [业务逻辑 §5](../design/business-logic.md)、[TDengine 超级表迁移](../design/tdengine-stable-migration.md)、[压测复盘](../operations/load-test-debugging.md) |
| 动机 | 单线程 MQTT 消费 + 逐条 PG/TDengine 调用，200 msg/s 已接近上限；写失败仅记日志丢数据 |

## 背景与现状

当前平台消费链路为**单线程串行**（`internal/platform/mqtt/service.go`）：

```
paho 回调（单 goroutine）→ ProcessMessage → GetProductByKey(PG) → AppendTelemetry(逐条 INSERT) → evaluateRules
```

瓶颈三处：
1. **串行**：paho 默认单 goroutine 派发，回调同步处理，一次只能处理一条，吞吐 = 1/单条延迟
2. **每消息一次 PG 查询**：`GetProductByKey` 取物模型做校验，产品为读多写少数据
3. **每消息一次 TDengine HTTP 写入**：固定开销（连接/解析/落盘）摊不到多条上

写失败兜底仅"记日志 + `IncMQTTErrors()`"，消息丢失（paho 回调返回时 QoS1 已 ack，broker 不再重投）。

## 目标架构

把消费路径改为流水线，**回调薄化、处理并行、写入成批、失败可重放**：

```
paho 回调 → route(按 device_id 哈希分片入队，立即返回)
                  ↓
         N 个 worker goroutine（每分片一个，同设备保序）
                  ↓
      产品缓存（免 PG 查询）→ 校验 → 批量写入器（攒批）
                  ↓
        TDengine 批量 INSERT（重试 + 隔离重放）
```

## 组件设计

### 1. 路由分片 `route`

- 回调只做：`ParseDeviceTopic` 取 `device_id` → `hash(device_id) % N` 选队列 → 入队**非阻塞**（队列满则丢弃 + 指标 `IncMQTTDropped`）
- 回调立即返回，MQTT 接收永不阻塞；背压由有界队列天然承担
- 同设备永远进同一队列 → 同设备消息（telemetry / event / command/reply / shadow/reported）顺序保持

### 2. Worker 池

- N 个 goroutine，各自 `for in := range queue` 消费，同步执行 `ProcessMessage`
- 并发度 = N，吞吐 ≈ N × 单 worker 吞吐
- 规则窗口状态（`shouldTrigger`）已由 `s.mu` 保护，多 worker 并发安全

### 3. 产品缓存 `productCache`

- 进程内 `map` + TTL（默认 60s），`loader` 包装 `GetProductByKey`
- 命中零网络开销；未命中才查 PG 并回填；产品更新后 TTL 过期自然失效
- 多实例各持一份，读多写少可接受短暂不一致

### 4. 批量写入器 `batchWriter`

- 攒批触发：`len(buf) >= size`（默认 200）→ 立即刷；后台 ticker 每 `interval`（默认 200ms）刷一次 → 有界延迟
- 刷写：单条语句多表插入
  `INSERT INTO t_a USING ... TAGS(...) VALUES (...) (...) t_b USING ... TAGS(...) VALUES (...)`
- 幂等保证：子表以 `(device_id, ts)` 为主键，同 ts 覆盖同 payload → **重试/重放无副作用**，语义收敛为"对相同数据恰好一次"

### 5. 失败兜底（分级）

| 层 | 机制 | 说明 |
| --- | --- | --- |
| 0 | 入队前防御 | worker 校验 payload 长度(≤ NCHAR 4096) / ts，坏行隔离不进批 |
| 1 | 内存重试 | 单次刷写失败退避重试（默认 3 次：50ms→200ms→800ms） |
| 2 | 隔离重放 | 重试耗尽 → 批次进内存 `pending` 队列，后台 ticker 慢速重试；仍失败保持队列 |
| 3 | 有界丢弃 | `pending` 超过上限丢弃最老批次 + `IncMQTTDropped`，不拖死流水线 |

- 读路径永远只返回 TDengine 已提交的行 → 返回的一定是真实数据，只是最多滞后一个攒批窗口
- Redis DLQ（跨实例持久化死信）列为后续扩展，当前为进程内隔离

## 一致性语义

- 写路径：从"同步承诺"改为"异步可追溯"——回调返回仅表示"已入流水线"；成败经 `IncTelemetryBatchFlushed / Failures / Retries` 与日志可观测
- 读路径：**有界最终一致**（滞后 ≤ 攒批窗口）；最新值快照如需实时可走 Redis 快照通道（现有影子缓存模式）
- 规则评估时机：在样本校验通过、入批后立即执行（而非等落库确认）——告警依据的是真实上报值，TDengine 写入失败不影响告警正确性

## 配置项

| 环境变量 | 默认 | 说明 |
| --- | --- | --- |
| `IOT_MQTT_WORKERS` | 4 | worker / 分片数 |
| `IOT_MQTT_QUEUE_SIZE` | 1024 | 每分片队列容量 |
| `IOT_TELEMETRY_BATCH_SIZE` | 200 | 批量刷写行数 |
| `IOT_TELEMETRY_BATCH_INTERVAL_MS` | 200 | 批量刷写最大间隔 |
| `IOT_PRODUCT_CACHE_TTL_SECONDS` | 60 | 产品缓存 TTL |

## 指标（Prometheus 新增）

- `iot_platform_mqtt_dropped_total`：队列满丢弃
- `iot_platform_telemetry_batch_flushed_total`：批量落库行数
- `iot_platform_telemetry_batch_retries_total`：批量重试次数
- `iot_platform_telemetry_batch_failures_total`：批量失败（进入隔离）次数

## 验收标准

- `go build ./... && go vet ./... && go test ./...` 全绿
- 串行消费路径（未启动流水线时直调 `ProcessMessage`）保持同步语义，现有测试不回归
- 批量刷写、产品缓存命中、失败重试与隔离重放均有单测
- 压测回归（后续人工执行）：200 msg/s 上限提升，落库率不下降

## 明确不做（后续）

- Redis/Kafka 持久化 DLQ（跨实例死信）
- 遥测链路 Kafka 削峰
- 冷热分层 / 归档

## 变更记录

| 时间 | 内容 |
| --- | --- |
| 2026-08-04 | 初稿（实现前设计） |
| 2026-08-05 | 实现完成，转 Baseline；补充"实现记录与踩坑" |

## 实现记录与踩坑

### 最终实现（与设计的差异）

1. **回调计数语义迁移**：`IncMQTTMessages` 从 `ProcessMessage` 移到 `route`。
   回调不再同步处理，计数含义变为"进入消费流水线"，直调 `ProcessMessage` 的测试不计数。
2. **batchWriter 创建时机**：在 `startConsumers()` 中创建（生产环境 `Start` 后才有）。
   未启动流水线时直调 `ProcessMessage` 走原有同步 `AppendTelemetry` 回退，
   保证"串行路径不回归"——现有单测无需改动。
3. **批量 INSERT 结构**：按 `device_id` 分组，一条语句多段 `USING ... TAGS ... VALUES`，
   同设备多行用多个 VALUES 元组；空批/全无效样本不发请求。
4. **规则评估时机**：改为样本校验通过、入批后**立即**评估（不等落库确认）。
   告警依据的是真实上报值，与 TDengine 写入解耦；代价是 TDengine 最终写失败时告警仍已产生（可接受）。
5. **有界最终一致**：Web 查询只返回 TDengine 已提交行，滞后 ≤ 攒批窗口；快照/实时场景走 Redis 快照通道。

### 踩坑记录

| 坑 | 现象 | 修复 |
| --- | --- | --- |
| shutdown 死锁 | `stopConsumers` 先 `Wait` 一个同时含 worker 与 batchWriter 的 WaitGroup；batchWriter 只在 `cancel` 后退出，而 `cancel` 在 `Wait` 之后 → 永久阻塞 | 拆成 `workerWG` / `batchWG` 两个 WaitGroup；顺序：close 队列 → wait worker（先排空 Add）→ cancel → wait batchWriter（flush 剩余） |
| 直调测试不落库 | batchWriter 在构造函数创建时，直调 `ProcessMessage` 的现有测试只 `Add` 不刷 → 查询为空 | batchWriter 推迟到 `startConsumers` 创建；未启动流水线走同步回退 |
| 产品缓存 nil 守护 | 测试中 `repos.Products == nil` 时 `GetProductByKey` 方法值为 nil | 构造时仅在 `repos.Products != nil` 才建缓存 |
| 隔离重放顺序 | pending 恢复后若乱序重写，虽因子表 `(device_id, ts)` 幂等无害，仍保持"先失败先重试" | `retryPending` 把失败批次**前插**回 pending 头 |

### 关键收获

- **幂等是兜底的地基**：子表以 `(device_id, ts)` 为主键、同 ts 覆盖同 payload，
  使"内存重试 + 隔离重放"无需去重，语义收敛为"对相同数据恰好一次"。
- **回调薄化是并发的前提**：paho 回调一旦同步干活就被拖死；路由入队即返，
  慢的 I/O 全部移进 worker 池，MQTT 接收永不阻塞。
- **关闭顺序 = 数据一致性**：先排空 worker 再停批量器，避免 shutdown 时新增行漏刷。
