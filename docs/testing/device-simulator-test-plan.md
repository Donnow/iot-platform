# 设备模拟器测试合同

| 项目 | 内容 |
| --- | --- |
| 版本 | v0.1 |
| 日期 | 2026-08-01 |
| 状态 | Draft |
| 关联需求 | [IoT 平台需求规格说明书](../requirements/iot-platform-srs.md) |

本文件用于在实现设备模拟器前锁定可观察行为。测试只依赖模拟器对 MQTT Broker 的行为，不依赖 PostgreSQL、Redis 或 TDengine；后端联调另用 EMQX 集成测试验证。

## 1. 构建目标

模拟器必须提供四种设备行为：

- `temperature`：周期上报 `temperature` 和 `humidity`。
- `smoke`：周期上报 `smoke_level`，超过配置阈值时上报告警事件。
- `door`：接收 `open`、`close`，上报 `door_status`。
- `air-conditioner`：接收 `setTemp`，维护 `target_temp`、`current_temp`、`mode`。

每个模拟设备都必须具备独立身份、独立 MQTT 会话和独立生命周期。模拟器必须支持确定性测试，因此实现需要允许注入随机数源、时钟和 MQTT 客户端工厂；不得在测试中依赖真实等待或不可控随机数。

## 2. 协议固定项

| 项目 | 测试合同 |
| --- | --- |
| client_id | 使用设备 `device_id`，同一批设备不重复 |
| username | 使用设备 `device_id` |
| password | 使用设备注册得到的 `device_secret` |
| 上报 QoS | 所有设备上行消息为 QoS 1 |
| 上报格式 | JSON，包含毫秒级 `ts` 和 `values` |
| 设备上行 | `telemetry`、`event`、`command/reply`、`shadow/reported` |
| 平台下行 | `command`、`shadow/desired`、`ota` |
| topic | 严格符合 `devices/{product_key}/{device_id}/{suffix}` |
| 停止行为 | 主动正常断开不伪造异常断线；异常断线依赖 MQTT 遗嘱发布 `status: offline` |

## 3. 单元和协议测试

这些测试必须不启动 EMQX，使用 fake broker 记录连接、订阅和发布调用。

| ID | 场景 | 通过标准 |
| --- | --- | --- |
| CFG-01 | 默认配置 | 默认设备类型、上报间隔、波动范围和 broker 地址可生成有效配置 |
| CFG-02 | 参数覆盖 | `product`、`count`、`interval`、`fluctuation`、`type`、broker 地址和凭据覆盖默认值 |
| CFG-03 | 非法参数 | 空 product、空 device secret、count 小于 1、interval 小于等于 0、负波动范围、未知设备类型均拒绝启动并给出明确错误 |
| CFG-04 | stress 参数 | `-stress -count=1000` 生成 1000 个独立设备，普通模式不因为 stress 默认值改变行为 |
| CFG-05 | 身份生成 | 未指定设备 ID 时生成唯一且可复现格式的 ID；指定 ID 时保留用户输入并拒绝重复 |
| PROTO-01 | 温湿度 topic | 发布到 `devices/{pk}/{id}/telemetry`，topic 中 product key 和 device ID 正确 |
| PROTO-02 | 消息 envelope | 每条 telemetry 有有效 `ts`；`values` 非空；JSON 可反序列化；不包含 NaN 或 Infinity |
| PROTO-03 | QoS | telemetry、event、command reply、shadow reported 均以 QoS 1 发布 |
| PROTO-04 | 订阅集合 | 只订阅自身的 command、shadow desired 和 ota topic，不订阅其他设备 topic |
| PROTO-05 | 下行 JSON | command、shadow、ota payload 非法 JSON 时不会 panic，记录错误并继续运行 |
| PROTO-06 | topic 隔离 | 修改另一个 device ID 的下行消息不会改变当前设备状态，也不会产生响应 |

## 4. 四种设备行为测试

每组测试都使用固定时钟和固定随机序列，先触发一次周期 tick，再检查 fake broker 的发布记录。

| ID | 设备 | 场景 | 通过标准 |
| --- | --- | --- | --- |
| DEV-01 | 温湿度 | 周期上报 | 每个 tick 恰好一条 telemetry，包含 `temperature`、`humidity` |
| DEV-02 | 温湿度 | 边界与波动 | temperature 始终在 15-45，humidity 始终在 30-90；波动范围为 0 时值保持稳定 |
| DEV-03 | 烟感 | 周期上报 | 每个 tick 包含 `smoke_level`，值始终在 0-100 |
| DEV-04 | 烟感 | 阈值未越过 | smoke level 小于等于阈值时不重复产生告警事件 |
| DEV-05 | 烟感 | 阈值越过 | 从阈值以下进入阈值以上时产生一次 `event_type=alarm` 事件；持续超阈值不重复刷屏 |
| DEV-06 | 门禁 | open/close | 收到合法 command 后更新状态并发布 telemetry；`door_status` 只能是 `open` 或 `closed` |
| DEV-07 | 门禁 | 非法 command | 未知 method 或非法 params 发布 code 非 0 的 command reply，不改变门状态 |
| DEV-08 | 空调 | setTemp | 解析 target 和 mode，更新 target_temp/mode，并发布成功 command reply |
| DEV-09 | 空调 | 温度变化 | 后续 telemetry 同时包含 current_temp、target_temp、mode；current_temp 向 target_temp 收敛且不越界 |
| DEV-10 | 空调 | 非法目标温度 | 缺少 target、类型错误或超出设备允许范围时返回失败，不改变原状态 |

## 5. 连接、遗嘱和重连测试

| ID | 场景 | 通过标准 |
| --- | --- | --- |
| LIFE-01 | 首次连接 | 使用正确 client_id、username、password 连接，连接成功后创建全部下行订阅 |
| LIFE-02 | 遗嘱 | CONNECT 包携带自身 `event` topic 的 `status: offline` 遗嘱，QoS 1；不同设备不共用 topic |
| LIFE-03 | 异常断线 | broker 模拟连接丢失后停止旧会话的周期任务，按退避策略重连，不产生重复 goroutine |
| LIFE-04 | 重连成功 | 重连后恢复下行订阅和 telemetry 上报；设备身份和状态不被重置 |
| LIFE-05 | 多次断线 | 连续断线/恢复不会重复订阅、重复发布遗嘱或泄漏资源；测试结束后所有设备都能停止 |
| LIFE-06 | 主动停止 | 收到 context cancel 后停止 ticker、取消订阅并主动断开；不把主动停止误报成异常断线 |

## 6. 下行指令、影子和 OTA 测试

| ID | 场景 | 通过标准 |
| --- | --- | --- |
| DOWN-01 | 通用 command 成功 | 收到合法 command 后，在 command/reply 回传同一 command_id、code=0 和 message |
| DOWN-02 | command 幂等 | 同一 command_id 重复投递不会重复执行状态变更；回复结果稳定 |
| DOWN-03 | shadow desired | 收到 desired 后更新本地 desired 状态，并在 shadow/reported 回报设备实际状态 |
| DOWN-04 | shadow 非法数据 | 非法 desired 不改变本地状态，不 panic，不发布伪造 reported |
| DOWN-05 | OTA 通知 | 收到目标版本、URL、MD5 后按 `downloading -> installing -> success` 上报 `ota_progress` |
| DOWN-06 | OTA 失败 | URL、MD5 或消息字段非法时上报 `stage=failed` 和非空错误信息 |
| DOWN-07 | OTA 重复通知 | 相同版本重复通知不会并发执行两次升级；最终只产生一条完成结果 |
| DOWN-08 | 多下行并发 | command、shadow、ota 同时到达时状态互不覆盖，所有回复仍引用正确的设备和 command_id |

## 7. 压力和资源测试

压力测试使用 fake broker 做快速协议验证，再用 EMQX 做一次真实连接验证。

| ID | 场景 | 通过标准 |
| --- | --- | --- |
| LOAD-01 | 1000 并发连接 | `-stress -count=1000` 创建 1000 个连接请求，所有 device_id 唯一，无 panic、data race 或死锁 |
| LOAD-02 | 周期吞吐 | 1000 个设备按 5 秒间隔各上报一条，10 个周期内消息数和设备分布正确 |
| LOAD-03 | 连接恢复 | 随机断开 10% 设备后全部最终重连，未断设备继续上报 |
| LOAD-04 | 停止与泄漏 | 压测停止后 goroutine、ticker、MQTT client 均释放；重复启动停止 3 次结果一致 |
| LOAD-05 | 真实 EMQX | 单节点 EMQX 接受 1000 个模拟设备连接，设备能上报并接收至少一条 command；记录连接耗时和消息吞吐 |

## 8. 端到端验收测试

以下测试标记为 `integration`，默认不在普通单元测试中运行：

1. 注册四种产品和设备，启动模拟器，验证 EMQX 认证使用正确 device_id/device_secret。
2. 验证设备上线 webhook 后平台在 2 秒内返回 online，模拟器异常退出后遗嘱和 webhook 使平台进入 offline。
3. 平台下发 command，验证门禁/空调状态改变并在 1 秒内查到 command success。
4. 设备离线期间设置 shadow desired，重新连接后验证 5 秒内收到 desired 并上报 reported。
5. 平台发送 OTA，验证模拟器上报完整进度，最终任务统计为 success。
6. 模拟器遥测进入平台规则，验证烟感阈值或温度规则能触发告警，且不重复触发。

## 9. 实现前需要锁定的默认约定

SRS 没有完全定义以下行为。若没有额外指定，按右侧默认值实现，避免测试和实现互相猜测。

| 项目 | 默认约定 |
| --- | --- |
| credentials 来源 | CLI 接收单设备 secret 或 CSV/JSON 设备清单；压力模式从清单生成设备身份 |
| smoke 告警格式 | `event` topic，`event_type=alarm`，data 包含 `metric=smoke_level`、`value`、`threshold` |
| shadow reported | 设备收到 desired 后立即回报当前本地状态；desired 字段原样保留 |
| OTA 下载 | 第一阶段不下载真实文件，只校验字段并模拟进度 |
| command 失败 | method/params 校验失败返回 `code=1`；未知 command 不退出设备进程 |
| 空调范围 | target_temp 默认 16-30；mode 允许 `cooling`、`heating`、`auto`、`off` |
| 正常退出 | context cancel 后主动 DISCONNECT，不触发遗嘱；网络异常才依赖遗嘱 |
| 重连退避 | 1s、2s、4s...，上限 30s；测试通过注入时钟跳过真实等待 |
| 随机性 | 默认随机，测试可传 seed；波动范围表示单次变化的最大绝对值 |

## 10. 构建门槛

第一阶段模拟器合并前必须通过 `CFG-*`、`PROTO-*`、`DEV-*`、`LIFE-*`、`DOWN-*` 全部测试，并通过 `go test -race ./...`。`LOAD-*` 和真实 EMQX 测试作为 M1/M7 验收门槛，不应阻塞本地单元测试。

实现完成后，测试命令应至少提供：

```bash
go test ./...
go test -race ./...
go test -tags=integration ./...
```
