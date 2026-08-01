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

服务入口为 `http://localhost:8080`，EMQX MQTT 端口为 `1883`，Dashboard
端口为 `18083`。Compose 使用持久化 volume 保存 PostgreSQL、Redis 和 TDengine
数据；当前 Go 服务的数据仓储仍使用内存实现，服务重启后业务数据会重置。

不启动 Compose 也可以直接运行服务，但必须确保 MQTT broker 可连接：

```bash
IOT_MQTT_BROKER_URL=tcp://localhost:1883 go run ./cmd/platform
```

服务会先提供 HTTP，再以指数退避重试 MQTT 连接；收到 SIGINT 或 SIGTERM 时会
停止消费、断开 MQTT 并优雅关闭 HTTP 连接。

## 健康与指标

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/metrics
```

`/metrics` 输出 Prometheus text format，当前包含 HTTP 请求、MQTT 消息、MQTT
处理错误、规则命中和告警创建计数器。

## EMQX 回调

EMQX 的 HTTP authentication 应指向 `/internal/emqx/auth`，生命周期 webhook
应指向 `/internal/emqx/webhook`。这两个接口是内部入口，不使用平台 JWT；部署时
应通过 Docker 网络、反向代理或网络策略限制其来源，仅允许 EMQX 访问。

平台收到设备上线事件后会更新设备状态；如果影子存在未同步的 desired，会立即
向设备 topic 补发 desired。

## 排障

结构化日志输出到标准输出。重点字段包括 `addr`、`device_id`、`topic`、`error`
和 `backoff`。如果平台持续记录 MQTT start failure，先检查 `IOT_MQTT_BROKER_URL`
和 EMQX 健康状态，再检查设备认证回调是否能访问平台 HTTP 端口。
