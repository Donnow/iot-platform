# 后端平台运行手册

| 项目 | 内容 |
| --- | --- |
| 版本 | v0.1 |
| 日期 | 2026-08-01 |
| 状态 | Draft |
| 关联接口 | [后端接口与消息契约](../api/backend-api.md) |

## 本地启动

复制环境示例后启动完整基础设施：

```bash
cp .env.example .env
docker compose up --build
```

管理台入口为 `http://localhost:3000`，后端入口为 `http://localhost:8080`，EMQX MQTT 端口为 `1883`，Dashboard
端口为 `18083`，WebSocket MQTT 端口为 `8083`。Compose 使用持久化 volume 保存 PostgreSQL、Redis 和 TDengine
数据；Go 服务默认使用 PostgreSQL、Redis 和 TDengine 持久化仓储。

API 文档可从 `http://localhost:8080/docs` 打开，机器可读契约在
`http://localhost:8080/openapi.yaml`。

不启动 Compose 也可以使用内存仓储快速运行服务：

```bash
IOT_STORAGE_MODE=memory IOT_MQTT_BROKER_URL=tcp://localhost:1883 go run ./cmd/platform
```

直接使用持久化模式时，还需要本机 PostgreSQL、Redis 和 TDengine 已启动，并通过
`IOT_DATABASE_URL`、`IOT_REDIS_ADDR` 和 `IOT_TDENGINE_URL` 指向它们。

服务会先提供 HTTP，再以指数退避重试 MQTT 连接；收到 SIGINT 或 SIGTERM 时会
停止消费、断开 MQTT 并优雅关闭 HTTP 连接。

## 健康与指标

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/metrics
```

`/metrics` 输出 Prometheus text format，当前包含 HTTP 请求、MQTT 消息、MQTT
处理错误、规则命中和告警创建计数器。

平台启动前会等待 PostgreSQL、Redis 和 TDengine 完成连接及 schema 初始化；Compose
通过 healthcheck 控制启动顺序。Redis 保存在线状态 TTL 和影子缓存，PostgreSQL 保存
产品、设备、规则、告警、指令和 OTA 元数据，TDengine 保存遥测 JSON payload。

管理台如需 MQTT.js 实时状态，将前端构建变量设置为
`VITE_MQTT_WS_URL=ws://localhost:8083/mqtt`，并提供具备状态 topic 订阅权限的
`VITE_MQTT_USERNAME` 与 `VITE_MQTT_PASSWORD`。未配置时管理台继续使用 HTTP 刷新；
平台在设备生命周期变化时向 `devices/{product_key}/{device_id}/status` 发布 retained
状态消息。

## EMQX 回调

Compose 已将 EMQX HTTP authentication 指向 `/internal/emqx/auth`，并将 HTTP
authorization 指向 `/internal/emqx/acl`；生命周期 webhook 应指向
`/internal/emqx/webhook`。这些接口是内部入口，不使用平台 JWT；部署时
应通过 Docker 网络、反向代理或网络策略限制其来源，仅允许 EMQX 访问。

平台收到设备上线事件后会更新设备状态；如果影子存在未同步的 desired，会立即
向设备 topic 补发 desired。设备上线时还会补发该设备所有未完成的 OTA 任务；
任务创建时只通知当前在线的目标设备。

## OTA 联调

先创建产品和设备，再上传固件元数据并创建任务：

```bash
curl -X POST http://localhost:8080/api/firmwares \
  -H 'Content-Type: application/json' \
  -d '{"product_key":"temperature","version":"1.2.3","md5":"0123456789abcdef0123456789abcdef","file_url":"https://firmware.example/temperature.bin"}'
```

使用返回的 `id` 调用 `POST /api/ota/tasks`。任务详情中的 `summary` 展示各 OTA
阶段的设备数；设备模拟器会依次上报 downloading、installing 和 success。

## 排障

结构化日志输出到标准输出。重点字段包括 `addr`、`device_id`、`topic`、`error`
和 `backoff`。如果平台持续记录 MQTT start failure，先检查 `IOT_MQTT_BROKER_URL`
和 EMQX 健康状态，再检查设备认证回调是否能访问平台 HTTP 端口。
