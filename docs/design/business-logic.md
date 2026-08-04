# 平台业务逻辑详解

| 项目 | 内容 |
| --- | --- |
| 版本 | v1.0 |
| 日期 | 2026-08-02 |
| 状态 | Baseline |
| 关联文档 | [架构](../design/architecture.md)、[接口契约](../api/backend-api.md)、[需求规格](../requirements/iot-platform-srs.md) |

本文档从业务视角描述平台全部业务流程与实现细节，涵盖产品、设备、遥测、规则告警、
影子、指令、OTA、模拟器与前端控制台，以及存储策略和容错机制。与代码实现一一对应。

---

## 1. 系统组成

| 组件 | 职责 | 端口 |
| --- | --- | --- |
| platform（Go） | 管理 API、EMQX 认证/ACL 回调、MQTT 消费、规则引擎 | 8080 |
| emqx（5.7.2） | MQTT Broker：连接、认证、ACL、消息路由、遗嘱 | 1883 / 8083 / 18083 |
| postgres（16） | 元数据权威存储：产品、设备、规则、告警、指令、影子、固件、OTA | 内部 |
| tdengine（3.x） | 遥测时序存储（动态 JSON payload） | 内部 |
| redis（7） | 在线状态缓存（TTL 10 分钟）、影子缓存 | 内部 |
| devicesim（Go） | 模拟 4 类设备行为，驱动全部业务链路 | — |
| frontend（Vue 3） | 运维控制台：总览/设备/告警/规则/产品/OTA | 3000 |

## 2. 核心业务对象与状态机

| 对象 | 状态 | 说明 |
| --- | --- | --- |
| 设备 | `inactive` → `online` ↔ `offline` → `deleted` | 注册后 inactive；认证通过转 online；遗嘱/断线转 offline；软删除 |
| 指令 | `pending` → `success` / `failed` / `timeout` | 下发即 pending；设备回复 code=0 转 success，非 0 转 failed；查询时超过 30s 仍 pending 惰性转 timeout |
| 告警 | `active` → `resolved` | 规则触发创建；仅人工解除（必须填备注），条件恢复不自动解除 |
| OTA 任务设备进度 | `pending` → `downloading` → `installing` → `success` / `failed` | 设备经 event 上报阶段与百分比 |
| 影子 | desired / reported / delta 三份数据 | delta = desired 与 reported 的差异，读取时计算 |

## 3. 业务链路明细

### 3.1 产品与物模型

**创建产品**（`POST /api/products`）
- 校验：名称必填、设备类型 ∈ {sensor, actuator, composite}、属性名唯一且不含 MQTT 通配符、属性类型 ∈ {int, float, bool, string}、min ≤ max
- `product_key` 可选，缺省自动生成 `pk-<unixnano>`，全局唯一（DB 唯一约束）
- 属性（物模型）随产品创建写入 `product_properties` 表，事务保证原子性

**物模型的业务作用**：设备上报遥测时按产品物模型做**类型与范围校验**——未定义的属性、
类型不符、超出 [min, max] 的数据被拒绝入库（不计入错误，记 MQTT 处理错误日志）。

### 3.2 设备生命周期

**注册设备**（`POST /api/devices`）
- 校验：product_key 与名称必填；device_id 可选（缺省生成 `device-<unixnano>`）
- `device_secret` 缺省自动生成 32 位十六进制随机串；**仅在创建响应中返回一次**，后续查询不包含
- 初始状态 `inactive`

**认证接入**（EMQX → `POST /internal/emqx/auth`）
- 设备以 device_id 作为 MQTT clientid 和 username、device_secret 作为 password 连接
- 平台校验：设备存在 + 未删除 + 密钥匹配（明文比对）；通过后返回 7 条设备级 ACL：
  - publish：`telemetry`、`event`、`command/reply`、`shadow/reported`
  - subscribe：`command`、`shadow/desired`、`ota`
- 失败返回 `{"result":"deny"}`，EMQX 回 CONNACK 0x05（Not Authorized）
- 平台内部 MQTT 服务账号（`IOT_MQTT_USERNAME/PASSWORD`）走独立 ACL：只允许订阅 4 个
  共享上行 topic、发布 command/ota/status/shadow-desired 下行 topic

**在线状态（上线）**：认证回调同时充当上线信号——认证通过且返回 ACL 的设备，延迟 1 秒
执行上线逻辑（见 3.5 与 3.7 的补发机制），更新 PG 状态 + Redis 在线缓存（`device:online:<id>`，
TTL 10 分钟）+ 发布 retained `devices/{pk}/{id}/status` 消息。

**在线状态（下线）**：设备连接异常断开时，EMQX 发布**遗嘱消息**到设备 `event` topic：
`{"status":"offline","ts":<毫秒时间戳>}`。平台消费后标记 offline。遗嘱带时间戳，
与设备最近一次上线时间（last_online）比较，**早于 last_online 5 秒以上的遗嘱视为
旧连接迟到消息而忽略**，防止"旧连接遗嘱覆盖新连接在线状态"的竞态。

**删除设备**：软删除（status=deleted），删除后认证被拒、列表不再返回；记录保留供审计。

### 3.3 遥测数据链路

```
设备 publish devices/{pk}/{id}/telemetry (QoS1, {"ts":ms,"values":{...}})
  → EMQX 共享订阅 $share/platform/devices/+/+/telemetry
  → 平台 ProcessMessage
  → 物模型校验（类型 + 范围，见 3.1）
  → TDengine 写入（REST SQL，payload 存 JSON，转义防注入）
  → 规则评估 evaluateRules（见 3.4）
```

**查询**（`GET /api/devices/:id/telemetry`）
- 参数：`metric`（按属性过滤）、`from`/`to`（Unix 秒）、`interval` ∈ {raw, 1m, 5m, 1h}、
  `limit`（上限 10000）
- 注意：结果按时间**升序**返回，`limit` 截取的是**最早的 N 条**；查看最新数据需配 `from`
- 聚合在应用层完成：按时间桶（1m/5m/1h）截断，数值属性取均值、非数值取最后值

**快照**（`GET /api/devices/:id/snapshot`）：遍历遥测取每个属性时间戳最新的样本。
（SRS 要求走 Redis 缓存 <50ms，当前实现为全量扫描取最新，属已知偏差）

### 3.4 规则与告警

**规则定义**（`POST /api/rules`，绑定产品，对产品下所有设备生效）
- 条件：属性名 + 运算符（`> < >= <= == !=`）+ 阈值 + 持续时间（秒，0 为立即触发）
- 动作：`alarm`（生成告警）或 `command`（下发指令，需在 action_params 配置 method/params）
- 创建即生效：每次遥测实时查询该产品规则，无需重启

**持续时间窗口**（进程内状态机，key=`deviceID + "\x00" + ruleID`）
- 条件不匹配 → 删除窗口状态（计时重置）
- 匹配但未持续够 duration → 记录起始时间继续等待
- 持续够且本窗口未触发 → 触发动作，标记 triggered（同规则同设备只触发一次，
  直到条件断开才允许再次触发）
- 窗口状态存内存，进程重启后重置（SRS 5.2 明确允许）

**告警**：触发时记录设备、规则、触发值、触发时间；仅能人工解除
（`PUT /api/alarms/:id/resolve`，必须填处理备注）。

### 3.5 设备影子

```
PUT /api/devices/{id}/shadow/desired  {targetTemp: 26}
  → 合并写入 desired（PG UPSERT + Redis 缓存）
  → 发布 MQTT devices/{pk}/{id}/shadow/desired（payload 为 desired 对象）

设备收到后执行并上报 devices/{pk}/{id}/shadow/reported  {"reported":{...}}
  → 平台合并写入 reported
  → delta 在读取时计算：desired 中与 reported 不一致的键
```

**离线补发**：设备上线时（认证回调 → 上线逻辑）若 delta 非空，自动重新发布 desired。
**Redis 兜底**：影子读取优先 Redis（`device:shadow:<id>`），miss 时回源 PG 并回填。

### 3.6 指令下发

```
POST /api/devices/{id}/commands  {"method":"open","params":{}}
  → 生成 command_id，状态 pending，立即返回 202（不阻塞等设备）
  → 发布 devices/{pk}/{id}/command  {"command_id","method","params"}

设备执行后回复 devices/{pk}/{id}/command/reply  {"command_id","code","message"}
  → code=0 → success；非 0 → failed

GET /api/devices/{id}/commands/{command_id} 查询状态
  → 查询时若仍 pending 且创建超 30s，惰性标记 timeout（无后台扫描任务）
```

发布失败时指令标记 failed 并返回 502。规则引擎的 command 动作走同一链路
（建命令 → 发布 → 设备回复）。

### 3.7 OTA 固件升级

**固件登记**（`POST /api/firmwares`）：仅元数据——SemVer 版本号（正则校验）、
32 位十六进制 MD5（正则校验）、下载 URL；同一产品同一版本不可重复（DB 唯一约束）。

**创建升级任务**（`POST /api/ota/tasks`）
- 目标：`all`（分页拉全量设备 ID）或 `devices`（指定列表，校验去重与归属）
- 校验固件属于该产品、目标设备存在且属于该产品且未删除
- 事务写入 `ota_tasks` + `ota_task_devices`（每设备 stage=pending）
- **在线设备立即收到通知**（发布 `devices/{pk}/{id}/ota`）；离线设备在下次上线时
  由上线逻辑补发（查 `ListPendingOTA`：该设备所有未 success/failed 的任务）

**进度追踪**：设备上报 `event`（event_type=ota_progress，含 version/stage/progress）
→ 平台按"产品 + 设备 + 固件版本"匹配任务 → 更新设备进度与任务 summary。

### 3.8 设备模拟器

**四种设备行为**：

| 类型 | 上报属性 | 支持指令 | 影子 | OTA |
| --- | --- | --- | --- | --- |
| temperature | temperature / humidity（随机游走 15-45°C / 30-90%） | 无 | 忽略 | 支持 |
| smoke | smoke_level（随机游走 0-100，越阈值发 alarm 事件） | 无 | 忽略 | 支持 |
| door | door_status（closed/open） | open / close | 忽略 | 支持 |
| air-conditioner | current_temp / target_temp / mode | setTemp{target,mode} | targetTemp/mode | 支持 |

**运行机制**
- 连接：clientid=username=device_id、password=secret、**clean session**（见问题记录 11）、
  3 秒 keepalive、遗嘱 `{"status":"offline","ts":...}` 发到 event topic
- 断线指数退避重连（1s → 30s 上限）
- 指令幂等：按 command_id 缓存回复，重复指令直接重发缓存回复
- OTA：收到通知依次上报 downloading(0) → installing(50) → success(100)；非法通知回 failed
- 压测模式：`-stress -count=1000` 并发 1000 设备

### 3.9 前端控制台

- **总览**：在线设备/活跃告警/遥测样本/生效规则四个指标卡，ECharts 遥测曲线，设备脉搏列表
- **设备**：搜索 + 状态筛选；详情面板含影子 JSON 编辑保存、指令下发（1s 轮询状态）、删除
- **告警**：状态 Tab 筛选 + 一键解除（自动填备注）
- **规则**：列表展示；新建规则表单（注意：command 类规则的 action_params 无 UI，需 API 创建）
- **产品**：卡片 + 创建（物模型属性编辑器）
- **OTA**：固件登记（手动填 MD5/URL，无实际上传）+ 升级任务创建 + 阶段汇总
- **实时性**：配置 `VITE_MQTT_WS_URL` 时用 mqtt.js 订阅 `devices/+/+/status` 与
  `devices/+/+/event` 刷新设备状态；未配置时走 HTTP 刷新
- 认证：JWT 存 localStorage，401 时弹令牌输入框

## 4. 消息契约速查

```
上行（设备 → 平台）
  devices/{pk}/{id}/telemetry       {"ts":ms,"values":{...}}         QoS1
  devices/{pk}/{id}/event           {"ts":ms,"event_type":"ota_progress","data":{...}}
  devices/{pk}/{id}/command/reply   {"command_id","code","message"}
  devices/{pk}/{id}/shadow/reported {"reported":{...}}
下行（平台 → 设备）
  devices/{pk}/{id}/command         {"command_id","method","params"}
  devices/{pk}/{id}/shadow/desired  {desired 对象本身}
  devices/{pk}/{id}/ota             {"task_id","firmware_id","version","url","md5"}
广播（平台 → 控制台）
  devices/{pk}/{id}/status          {"device_id","status","ts"}     retained
遗嘱（EMQX → 平台，异常断开时）
  devices/{pk}/{id}/event           {"status":"offline","ts":ms}
```

平台消费端使用共享订阅 `$share/platform/devices/+/+/{telemetry|event|command/reply|shadow/reported}`，
支持多实例横向扩展。

## 5. 存储与缓存策略

| 数据 | 存储 | 策略 |
| --- | --- | --- |
| 产品/设备/规则/告警/指令/影子/固件/OTA | PostgreSQL | 权威存储，事务保证一致性 |
| 遥测 | TDengine | 超级表 `iot_telemetry.telemetry(ts, payload NCHAR(4096)) TAGS (device_id BINARY(128), product_key BINARY(128))`，每设备一个子表 `t_<md5(device_id) 前 8 字节 hex>`，批量写入，保留 3650 天 |
| 产品模型缓存 | 进程内存 | 消费端物模型校验用，TTL 60s，读多写少免每消息 PG 查询 |
| 设备在线状态 | Redis `device:online:<id>` | TTL 10 分钟，上线 SET / 下线 DEL |
| 影子缓存 | Redis `device:shadow:<id>` | 写入时更新，读取时优先缓存、PG 兜底 |
| 规则窗口状态 | 进程内存 | 重启重置（允许） |

遥测落库走 `INSERT INTO <子表> USING iot_telemetry.telemetry TAGS (...) VALUES (...)`
自动建子表（幂等、并发安全），写入与查询均按设备分区，`device_id`/`product_key`
作为标签随子表存储，查询结果由应用层回填。由单普通表迁移到超级表模型的步骤见
[tdengine-stable-migration.md](tdengine-stable-migration.md) 与
`scripts/tdengine-migrate.sh`。

MQTT 消费端为**分片 worker 池 + 批量写入**：paho 回调按 `device_id` 哈希入队即返，
N 个 worker 并行消费；遥测样本攒批后单语句多表写入，失败退避重试并隔离重放。
一致性为有界最终一致（滞后 ≤ 攒批窗口），详见
[persistence-spec.md](persistence-spec.md)。

## 6. 容错与一致性机制

| 机制 | 说明 |
| --- | --- |
| 平台启动退避重试 | 存储依赖与 MQTT 均指数退避（1s→15s），依赖未就绪不退出 |
| 平台订阅自愈 | 监听断线通道，连接恢复后自动重新订阅 4 个共享 topic（EMQX 重启不丢消费） |
| 上线补发延迟 | 认证回调后延迟 1s 执行补发，给设备留出完成订阅的时间 |
| 遗嘱时间戳去重 | 忽略早于 last_online 5s 以上的遗嘱，防止旧连接覆盖新状态 |
| 设备侧 clean session | 不恢复 broker 会话，避免 paho 会话残留卡死；离线交付由平台补发承担 |
| 指令发布失败兜底 | 发布失败即标记 failed 并返回 502 |
| 遥测校验拦截 | 不合规数据拒绝入库，不污染时序库 |

## 7. 已知限制与偏差

> 以下限制的修复任务、状态与验收口径统一维护在
> [TODO.md](../../TODO.md)（需求基线：
> [architecture-evolution-srs.md](../requirements/architecture-evolution-srs.md)）。
> 括号内为对应任务 ID。

1. 快照未走 Redis 缓存（SRS 要求 <50ms，当前全量扫描）→ `ARC-P1-05`
2. 指令超时为惰性标记（不查询不标记），无后台清理任务 → `ARC-P1-04`
3. 告警条件恢复不自动解除 → `ARC-P1-03`
4. 规则/产品无更新、删除、启停接口 → `ARC-P1-03`
5. 设备密钥明文存储与比对（未加盐哈希）→ `ARC-P0-04`
6. `/internal/*` 回调无 IP 白名单（文档要求部署层限制）→ `ARC-P0-02`
7. 角色仅有 admin 一种：`/api/auth/login` 签发的 JWT 携带 `role` claim，
   但接口层未做分级校验（SRS 要求的管理员/运维分级未实现）→ `ARC-P1-11`
8. EMQX 生命周期 webhook 端点保留，但本环境（OSS 规则引擎收不到 client 事件）由
   认证回调 + 遗嘱链路承担在线/离线检测；`emqx-init` 会幂等创建规则引擎桥接，
   供支持 Webhooks 的 EMQX 版本使用
