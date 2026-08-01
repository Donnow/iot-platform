# 后端接口与消息契约

| 项目 | 内容 |
| --- | --- |
| 版本 | v0.1 |
| 日期 | 2026-08-01 |
| 状态 | Draft |

## HTTP

除 `/healthz`、`/metrics` 和 `/internal/emqx/*` 外，`/api/*` 请求需要携带
`Authorization: Bearer <JWT>`。JWT 使用 HS256，签名密钥由 `IOT_JWT_SECRET` 提供。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/products` | 创建产品；未提供 `product_key` 时自动生成 |
| GET | `/api/products?page=1&page_size=20` | 分页查询产品 |
| POST | `/api/devices` | 注册设备并返回一次性 `device_secret` |
| GET | `/api/devices?product_key=&status=` | 分页查询设备 |
| GET | `/api/devices/:id` | 查询设备 |
| DELETE | `/api/devices/:id` | 软删除设备 |
| GET | `/api/devices/:id/telemetry` | 查询遥测；支持 `metric`、`from`、`to`、`limit` |
| GET | `/api/devices/:id/snapshot` | 查询每个属性的最新遥测 |
| GET | `/api/devices/:id/shadow` | 查询 desired/reported/delta |
| PUT | `/api/devices/:id/shadow/desired` | 更新 desired 并发布 MQTT 消息 |
| POST | `/api/devices/:id/commands` | 创建并下发指令，返回 202 |
| GET | `/api/devices/:id/commands/:command_id` | 查询指令状态 |
| POST | `/api/rules` | 创建规则 |
| GET | `/api/rules?product_key=` | 查询产品规则 |
| GET | `/api/alarms` | 按设备、产品、状态、时间和分页条件查询告警 |
| PUT | `/api/alarms/:id/resolve` | 使用 `{"note":"..."}` 解除告警 |

错误响应统一为 JSON：

```json
{"error":"Not Found","message":"not found"}
```

## MQTT

设备使用 QoS 1，topic 约定如下：

```text
devices/{product_key}/{device_id}/telemetry
devices/{product_key}/{device_id}/event
devices/{product_key}/{device_id}/command
devices/{product_key}/{device_id}/command/reply
devices/{product_key}/{device_id}/shadow/desired
devices/{product_key}/{device_id}/shadow/reported
devices/{product_key}/{device_id}/ota
```

遥测 payload 为 `{"ts":1722000000000,"values":{"temperature":21.5}}`。
平台会按照产品物模型校验字段类型和数值范围，并将通过校验的数据交给规则引擎。
命令回复 payload 为 `{"command_id":"...","code":0,"message":"ok"}`；
`code != 0` 会将命令标记为 failed。

平台消费者使用 shared subscription 订阅上行消息。设备认证回调为
`POST /internal/emqx/auth`，请求至少包含 `username`、`password`、`clientid`；
生命周期回调为 `POST /internal/emqx/webhook`，事件使用
`client.connected` 或 `client.disconnected`，并包含 `clientid`。

## 当前实现边界

当前可运行服务使用线程安全内存仓储完成端到端联调；PostgreSQL、Redis 和
TDengine 已纳入 Compose 基础设施与配置契约，但持久化仓储适配器仍是后续阶段。
