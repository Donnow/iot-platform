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

## 记录模板

记录测试开始时间、Compose 镜像版本、设备数量、设备在线数、MQTT 入站消息数、TDengine 写入数、错误数、平均/P95/P99 延迟和宿主机 CPU、内存、网络、文件描述符。使用 `docker compose logs emqx platform` 保存服务日志，使用 `curl http://localhost:8080/metrics` 保存平台计数器。

## 当前状态

仓库提供可复现的启动命令和记录模板，但当前工作区没有声称已完成 1000 设备/5000 msg/s 的实测报告。实测结果应以部署环境输出为准，并在本文件追加日期、环境和原始指标。
