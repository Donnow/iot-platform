# 设备模拟器运行手册

| 项目 | 内容 |
| --- | --- |
| 版本 | v0.1 |
| 日期 | 2026-08-01 |
| 状态 | Draft |
| 关联测试 | [设备模拟器测试合同](../testing/device-simulator-test-plan.md) |

## 启动

在仓库根目录执行：

```bash
go run ./cmd/devicesim \
  -broker tcp://localhost:1883 \
  -product-key demo-product \
  -type temperature \
  -count 2 \
  -interval 5s
```

支持的设备类型为 `temperature`、`smoke`、`door` 和 `air-conditioner`。每个设备使用独立的 client ID、username 和 MQTT 遗嘱；默认生成的凭据适合本地 fake broker 或开放测试 broker，接入平台时应使用平台注册得到的设备凭据。

## 使用设备凭据

JSON 文件格式：

```json
[
  {"device_id": "temp-001", "device_secret": "secret-001"},
  {"device_id": "temp-002", "device_secret": "secret-002"}
]
```

CSV 文件必须包含 `device_id` 和 `device_secret` 两列：

```csv
device_id,device_secret
temp-001,secret-001
temp-002,secret-002
```

启动时指定文件：

```bash
go run ./cmd/devicesim \
  -credentials ./devices.json \
  -product-key temperature \
  -type temperature \
  -count 2
```

`count` 不能超过凭据文件中的设备数量；设备 ID 不能重复，也不能包含 MQTT 通配符。

## 行为和消息

- 温湿度设备周期发布 `temperature` 和 `humidity`，范围分别为 15-45 和 30-90。
- 烟感设备周期发布 `smoke_level`；从阈值以下进入阈值以上时发布一次 `alarm` 事件。
- 门禁设备响应 `open` 和 `close` 指令，并发布 `door_status`。
- 空调设备响应 `setTemp`，参数为 `target` 和 `mode`，并让 `current_temp` 逐步收敛到目标值。
- 所有上行和下行 topic、payload、QoS 约定见 [IoT 平台需求规格说明书](../requirements/iot-platform-srs.md)。

## 压力模式

启动 1000 个模拟设备，每个设备每 5 秒上报一次：

```bash
go run ./cmd/devicesim \
  -stress \
  -count 1000 \
  -interval 5s \
  -product-key stress-product \
  -type temperature
```

压力测试前应确认 broker 的连接数、消息吞吐和文件描述符限制。完整压力验收使用测试合同中的 `LOAD-*` 和 EMQX 集成测试。

## 停止和重连

使用 `Ctrl-C` 或发送 `SIGTERM` 停止。正常停止会主动断开连接，不依赖遗嘱；网络异常会按 1s、2s、4s 的退避策略重连，最大退避 30s。设备异常断开时，遗嘱发布到自身的 `event` topic，payload 为 `{"status":"offline"}`。
