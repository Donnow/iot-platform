# 压力测试

| 项目 | 内容 |
| --- | --- |
| 版本 | v0.1 |
| 日期 | 2026-08-01 |
| 状态 | Draft |

## 目标

验证单机 Compose 环境能维持 1000 个模拟设备连接，并记录消息吞吐、连接建立结果和端到端处理延迟。SRS 的目标值是 5000 msg/s、遥测写入 P99 小于 100ms；没有真实运行记录前，不把目标值标记为已达成。

## 可复现流程

先启动完整基础设施，并确认服务健康：

```bash
docker compose up --build -d
docker compose ps
curl -fsS http://localhost:8080/healthz
```

设备模拟器的内置 stress 模式可以验证 broker 连接和上报行为：

```bash
go run ./cmd/devicesim \
  -stress -count 1000 -interval 5s \
  -broker tcp://localhost:1883 \
  -product-key stress-product -type temperature
```

该命令使用生成凭据，适用于开放 broker 或单独的 simulator 测试；要验证平台 HTTP Auth，必须先创建同产品的设备并将返回的 `device_id/device_secret` 写入凭据文件，再通过 `-credentials` 启动。API 需要管理员 JWT，仓库目前不提供登录端点，因此压测脚本应通过环境变量 `IOT_API_TOKEN` 注入已签发 token。

## 实测记录（2026-08-03）

### 环境

- 宿主机：macOS（OrbStack 容器运行时），单机 Compose 全栈
- 版本：EMQX 5.7.2 / PostgreSQL 16 / Redis 7 / TDengine 3.3.6.0 / Go 1.24
- 容器资源（稳态）：platform CPU ~5% / 内存 31MB；EMQX CPU ~28%；TDengine CPU ~9%

### 1000 台设备压测（模拟器 tick 相位修复后）

```bash
# 1. 注册设备并生成凭据（1000 台）
IOT_API_TOKEN=<jwt> ./scripts/load-test.sh -no-sim -count 1000

# 2. 启动 1000 台模拟器（每台 5s 一报）
go run ./cmd/devicesim -stress -count 1000 -interval 5s \
  -broker tcp://localhost:1883 -credentials ./creds-stress.json \
  -product-key stress-product -type temperature
```

| 指标 | 实测 | 说明 |
| --- | --- | --- |
| 在线设备 | 1001 台（1000 压测 + 1 探针）稳定 | 启动瞬间 ~10 台认证回调超时，退避重连后全部上线 |
| 上报速率 | 200 msg/s（1000 台 × 5s） | EMQX received 与平台消费计数一致（10s 窗口 +2008/+2009） |
| 平台处理 | 200 msg/s，0 错误 | mqtt_errors_total 全程无新增 |
| TDengine 落库 | ~150 msg/s（75%） | 剩余被 ts 主键覆盖吸收，见问题 16 |
| 端到端延迟 | P50 11ms / P95 18ms / P99 18ms / max 25ms | 探针：发布 → 平台消费 → 校验 → TDengine → 查询可见（20 样本，2 条因同 ts 覆盖不可见） |
| 无流量基线延迟 | P50 13ms / P99 26ms | 同链路，单台设备 |

### 问题与修复

压测中发现并定位的问题详见 [问题记录](../operations/issues-encountered.md)：

- **#16 模拟器 tick 同步导致遥测 ts 相同，TDengine ts 主键覆盖丢数据（已修复）**：
  修复前 1000 台落库率仅 ~17%；修复（ticker 随机相位偏移 0–1000ms）后 10 台验证
  落库率 35% → 95%
- **#17 1000 台同时连接时认证回调瞬时超时（启动风暴）**：退避重连后全部恢复
- **存储层单表 ts 主键（SRS 规划的超级表/子表未实现）**：同毫秒多设备上报仍可能
  相互覆盖，列为后续优化

### 与 SRS 目标的差距

SRS 目标为 5000 msg/s、遥测写入 P99 < 100ms。本次实测：

- 端到端 P99 18ms **达成**（1000 台/200 msg/s 负载下）
- 5000 msg/s 目标**未验证**：模拟器生成能力（1000 台 × 5s = 200 msg/s）与平台单线程
  消费模型（paho 回调串行 + 逐条 PG 查询 + 逐条 TDengine HTTP 写入）均不足以支撑；
  平台消费已达 ~200 msg/s 上限附近。达到 5000 msg/s 需要：消费端多 worker、
  产品查询缓存、TDengine 批量写入（或 Kafka 削峰），见遗留观察项

## 记录模板

记录测试开始时间、Compose 镜像版本、设备数量、设备在线数、MQTT 入站消息数、TDengine 写入数、错误数、平均/P95/P99 延迟和宿主机 CPU、内存、网络、文件描述符。使用 `docker compose logs emqx platform` 保存服务日志，使用 `curl http://localhost:8080/metrics` 保存平台计数器。

## 当前状态

仓库提供可复现的启动命令和记录模板，但当前工作区没有声称已完成 1000 设备/5000 msg/s 的实测报告。实测结果应以部署环境输出为准，并在本文件追加日期、环境和原始指标。
