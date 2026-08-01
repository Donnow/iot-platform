# 后端测试计划

| 项目 | 内容 |
| --- | --- |
| 版本 | v0.1 |
| 日期 | 2026-08-01 |
| 状态 | Draft |

## 测试分层

普通 `go test ./...` 覆盖设备、内存仓储、HTTP 路由、MQTT payload 解析、规则、影子和 OTA 进度汇总。测试使用内存仓储和记录型 publisher，不依赖外部服务。

`integration` 标签增加跨模块链路：HTTP 创建固件与 OTA 任务，MQTT 处理设备进度事件，再通过 HTTP 查询任务阶段汇总。

```bash
GOCACHE=/tmp/iot-perform-go-cache go test ./...
GOCACHE=/tmp/iot-perform-go-cache go test -race ./...
GOCACHE=/tmp/iot-perform-go-cache go test -tags=integration ./...
GOCACHE=/tmp/iot-perform-go-cache go vet ./...
GOCACHE=/tmp/iot-perform-go-cache go build ./...
```

## OTA 验收项

| 编号 | 验收内容 |
| --- | --- |
| OTA-BE-01 | 同一产品相同 SemVer 固件返回冲突 |
| OTA-BE-02 | 全部设备和指定设备两种目标范围均校验产品归属与重复设备 |
| OTA-BE-03 | 创建任务只向在线设备发布 OTA，离线设备保持 pending |
| OTA-BE-04 | 设备上线时补发 pending OTA；shadow desired 同时存在时两者都发布 |
| OTA-BE-05 | `ota_progress` 更新设备状态，并正确返回各阶段数量 |
| OTA-BE-06 | 非法阶段、非整数进度和超出 0-100 的进度被拒绝 |

## 运行边界

当前测试验证的是内存仓储的端到端联调。PostgreSQL、Redis 和 TDengine 已在 Compose 中声明，但业务仓储适配器尚未接入；外部 broker、数据库和 1000 设备压力测试属于部署环境验收，不由默认测试命令隐式启动。
