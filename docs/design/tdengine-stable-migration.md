# TDengine 超级表/子表改造设计

| 项目 | 内容 |
| --- | --- |
| 状态 | Design（2026-08-03） |
| 关联文档 | [业务逻辑 §5](../design/business-logic.md)、[压测复盘](../operations/load-test-debugging.md) |
| 动机 | 压测暴露的架构债（同毫秒 ts 主键覆盖）+ 查询无设备分区 |

## 背景与问题

当前 TDengine 为单普通表 `iot_telemetry.telemetry(ts, device_id, product_key, payload NCHAR(4096))`，`ts` 为唯一主键。压测（1001 台 / 200 msg/s）暴露：同毫秒多设备上报触发主键冲突，TDengine 静默覆盖旧值，落库率仅 75%（模拟器相位偏移修复后 95%）。根因：数据模型没有设备维度分区。此外按 device_id 查询为全表扫描，数据量增长后性能退化。计划文档（2026-08-02 规划稿）选型理由明确写了"超级表/子表模型"，第一版实现退化为单表，属交付偏差。

## 目标结构

```sql
CREATE DATABASE IF NOT EXISTS iot_telemetry KEEP 3650;

CREATE STABLE IF NOT EXISTS iot_telemetry.telemetry (
    ts TIMESTAMP,
    payload NCHAR(4096)            -- 保持 NCHAR，decode 逻辑尽量少改
) TAGS (
    device_id   BINARY(128),       -- 从列下沉为标签
    product_key BINARY(128)
);
```

每设备一个子表，子表名由 device_id 编码生成：`t_` + md5(device_id) 前 8 字节的 hex（共 16 字符，定长，避免表名超限与非法字符）。

## 改动清单

### internal/platform/storage/telemetry.go

1. **EnsureSchema**：改为 `CREATE STABLE IF NOT EXISTS ...`；新增结构校验（`DESCRIBE iot_telemetry.telemetry` 核对列与标签）。旧普通表与新超级表同名冲突，见迁移步骤。
2. **新增 `telemetryChildTable(deviceID string) string`**：md5 编码函数。
3. **AppendTelemetry**：改为 `INSERT INTO <子表> USING iot_telemetry.telemetry TAGS ('<device_id>', '<product_key>') VALUES (<ts>, '<payload>')`——自动建子表，幂等。
4. **QueryTelemetry**：改查子表（`SELECT ts, payload FROM <子表> WHERE ts ... ORDER BY ts ASC LIMIT n`），`device_id`/`product_key` 在代码中直接回填到结果。
5. **SnapshotTelemetry**：查子表 `ORDER BY ts DESC LIMIT N` 取最新样本，应用层逻辑保留。
6. **decodeTelemetryRow**：兼容新查询列（不再依赖 SELECT 返回 device_id/product_key 列）。

### 迁移脚本 scripts/tdengine-migrate.sh

1. `RENAME TABLE iot_telemetry.telemetry TO iot_telemetry.telemetry_legacy`（旧表与超级表同名冲突，必须先改名）
2. 建超级表（EnsureSchema 逻辑）
3. 重放：SELECT 旧表全部数据，按 device_id 分组，`INSERT ... USING ... TAGS` 写入新子表
4. 校验：设备数 / 每设备样本数 / 时间范围 对比，抽样逐条比对 ts+payload
5. 确认后 `DROP TABLE iot_telemetry.telemetry_legacy`（提供 `--drop-legacy` 开关，默认不删）

### 测试与文档

- 更新/新增 telemetry 单测（子表编码函数、写入/查询路径）
- 更新 `docs/design/business-logic.md` §5 存储策略表
- README 中涉及 TDengine 表结构的段落同步

## 验收标准

- `go build ./... && go test ./...` 全绿
- 迁移脚本幂等、可重跑、校验失败不切换
- 压测回归（后续人工执行）：1001 台 / 200 msg/s，落库率 ~100%，端到端 P99 不回退

## 约束与坑

- 旧普通表与新超级表同名 → 必须先 RENAME 再建超级表
- device_id 含 `-` 不能直接作表名 → 编码函数（本项目 device_id 可自定义，不能假设格式）
- payload 保持 NCHAR(4096)，物模型动态属性照旧存 JSON 文本
- 聚合保持应用层（aggregateTelemetry 不动）
- 自动建子表并发安全（TDengine 内部处理）；首次大规模设备上线有建表开销，可接受
- 旧表已被覆盖的数据无法恢复，重放只搬现存数据（文档如实说明）

## 明确不做（二期）

- payload 改 JSON 类型 + `INTERVAL` 聚合下推
- 批量写入 / 多 worker 消费（与"平台消费端多 worker"遗留优化同批）
- 保留策略分层 / 冷热归档
