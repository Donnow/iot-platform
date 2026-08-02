# 后端接口与消息契约

| 项目 | 内容 |
| --- | --- |
| 版本 | v0.1 |
| 日期 | 2026-08-01 |
| 状态 | Draft |

## HTTP

除 `/healthz`、`/metrics`、`/docs`、`/openapi.yaml`、`/api/auth/login` 和
`/internal/emqx/*` 外，`/api/*` 请求需要携带
`Authorization: Bearer <JWT>`。JWT 使用 HS256，签名密钥由 `IOT_JWT_SECRET` 提供。
OpenAPI 原始文档为 `/openapi.yaml`，Swagger UI 为 `/docs`；两者均不需要 JWT，便于
在部署环境中查看契约。

### 认证

- `POST /api/auth/login`：使用 `{username, password}` 换取 JWT。账号存储在
  PostgreSQL `users` 表（密码为 bcrypt 哈希），首次启动时按
  `IOT_ADMIN_USERNAME` / `IOT_ADMIN_PASSWORD` 自动创建管理员（已存在则跳过）。
- 响应 `{token, expires_in, role, username}`；`expires_in` 由
  `IOT_JWT_TTL_SECONDS` 控制（默认 3600）。
- 凭据错误统一返回 `401 invalid credentials`（用户不存在与密码错误响应一致，
  避免用户名枚举）；端点未配置（JWT 密钥/仓储缺失）返回 `501`。
- 本地开发也可用 `scripts/make-jwt.sh` 直接签发测试令牌（压测脚本使用）。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/auth/login` | 登录并签发 JWT（无需携带令牌） |
| POST | `/api/products` | 创建产品；未提供 `product_key` 时自动生成 |
| GET | `/api/products?page=1&page_size=20` | 分页查询产品 |
| POST | `/api/devices` | 注册设备并返回一次性 `device_secret` |
| GET | `/api/devices?product_key=&status=` | 分页查询设备 |
| GET | `/api/devices/:id` | 查询设备 |
| DELETE | `/api/devices/:id` | 软删除设备 |
| GET | `/api/devices/:id/telemetry` | 查询遥测；支持 `metric`、`from`、`to`、`interval`、`limit` |
| GET | `/api/devices/:id/snapshot` | 查询每个属性的最新遥测 |
| GET | `/api/devices/:id/shadow` | 查询 desired/reported/delta |
| PUT | `/api/devices/:id/shadow/desired` | 更新 desired 并发布 MQTT 消息 |
| POST | `/api/devices/:id/commands` | 创建并下发指令，返回 202 |
| GET | `/api/devices/:id/commands/:command_id` | 查询指令状态 |
| POST | `/api/rules` | 创建规则 |
| GET | `/api/rules?product_key=` | 查询产品规则 |
| GET | `/api/alarms` | 按设备、产品、状态、时间和分页条件查询告警 |
| PUT | `/api/alarms/:id/resolve` | 使用 `{"note":"..."}` 解除告警 |
| POST | `/api/firmwares` | 创建固件元数据；同一产品的版本不可重复 |
| GET | `/api/firmwares?product_key=` | 查询产品固件 |
| POST | `/api/ota/tasks` | 创建 OTA 任务并通知在线目标设备 |
| GET | `/api/ota/tasks?product_key=` | 查询 OTA 任务列表 |
| GET | `/api/ota/tasks/:id` | 查询任务明细、设备进度和阶段汇总 |

固件创建请求：

```json
{
  "product_key": "temperature",
  "version": "1.2.3",
  "md5": "0123456789abcdef0123456789abcdef",
  "file_url": "https://firmware.example/temperature-1.2.3.bin",
  "changelog": "修复传感器采样问题"
}
```

OTA 任务支持全部设备或指定设备。`target` 为 `all` 时忽略设备列表并选择
产品下全部未删除设备；`target` 为 `devices` 时必须提供
`target_device_ids`。未提供 `target` 时，根据是否提供设备列表自动选择这两种模式。

```json
{
  "product_key": "temperature",
  "firmware_id": "firmware-1",
  "target": "devices",
  "target_device_ids": ["temp-001", "temp-002"]
}
```

任务响应中的 `progress` 为每台目标设备的最新状态，`summary` 为按阶段统计的
设备数量，例如 `{"pending":1,"success":1}`。任务创建时只向在线设备立即发布；
离线设备在 EMQX 生命周期回调报告上线后补发。设备上报 `ota_progress` 后，平台
根据产品、设备和固件版本更新对应任务。

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
devices/{product_key}/{device_id}/status
```

遥测 payload 为 `{"ts":1722000000000,"values":{"temperature":21.5}}`。
平台会按照产品物模型校验字段类型和数值范围，并将通过校验的数据交给规则引擎。
命令回复 payload 为 `{"command_id":"...","code":0,"message":"ok"}`；
`code != 0` 会将命令标记为 failed。

OTA 下行 payload 为：

```json
{
  "task_id": "ota-1",
  "firmware_id": "firmware-1",
  "version": "1.2.3",
  "url": "https://firmware.example/temperature-1.2.3.bin",
  "md5": "0123456789abcdef0123456789abcdef"
}
```

OTA 进度事件沿用设备 `event` topic，格式为
`{"ts":1722000000000,"event_type":"ota_progress","data":{"version":"1.2.3","stage":"installing","progress":50}}`；阶段为
`downloading`、`installing`、`success` 或 `failed`，进度范围为 0 到 100。

平台消费者使用 shared subscription 订阅上行消息。设备认证回调为
`POST /internal/emqx/auth`，请求至少包含 `username`、`password`、`clientid`；
该回调同时作为上线信号——认证通过后平台延迟 1 秒执行上线逻辑（补发影子
desired 和未完成 OTA 通知）。生命周期 webhook 为 `POST /internal/emqx/webhook`，
事件使用 `client.connected` 或 `client.disconnected`，并包含 `clientid`。
设备异常断开时，EMQX 发布遗嘱消息到设备 `event` topic（
`{"status":"offline","ts":...}`），平台据此标记下线，并通过 `ts` 忽略
旧连接的迟到遗嘱。平台在生命周期变化时向 `status` topic 发布 retained 状态
消息，管理台可以通过 MQTT.js WebSocket 订阅该 topic 获取实时在线状态。

## 当前实现边界

平台默认使用 PostgreSQL、Redis 和 TDengine 持久化仓储；设置
`IOT_STORAGE_MODE=memory` 可切换为不依赖外部服务的内存仓储，用于快速开发和普通单测。
规则持续时间窗口仍是进程内状态，服务重启后按需求允许重置。
