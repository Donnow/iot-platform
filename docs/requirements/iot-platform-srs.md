# 智慧园区 IoT 设备管理平台 — 需求规格说明书

**版本：** v1.0  
**日期：** 2026-07-26  
**状态：** 基线

---

## 1. 项目背景

某园区管理公司需要统一管理园区内部署的各类传感器和执行器设备，现有方案依赖人工巡检，无法实时感知设备状态。本项目交付一套 IoT 设备管理平台，实现设备接入、数据采集、规则告警、远程控制的完整闭环。

**项目交付物：**
- 后端服务（Go）
- 设备模拟器（Go）
- 管理前端（Vue 3）
- 基础设施配置（Docker Compose）
- 技术文档（架构、API、部署）

---

## 2. 角色与使用场景

| 角色 | 描述 |
|------|------|
| 平台管理员 | 管理产品、设备、规则，查看全局统计 |
| 运维人员 | 查看设备状态、历史数据、告警，执行操作 |
| 设备（模拟器） | 上报数据、接收指令、响应 OTA |
| EMQX | 调用平台认证接口、推送设备生命周期事件 |

---

## 3. 系统架构约束

- MQTT Broker：EMQX 5.x，Docker 部署
- 后端语言：Go 1.22+
- 时序数据库：TDengine 3.x（遥测数据）
- 关系数据库：PostgreSQL 16（元数据、规则、告警）
- 缓存：Redis 7（设备在线状态、影子缓存）
- 前端：Vue 3 + ECharts（数据可视化）
- 部署：单机 Docker Compose，支持扩展为多节点

---

## 4. 功能需求

### 4.1 产品管理

**FR-P01 创建产品**  
管理员可创建产品，产品是设备的型号模板。  
字段：产品名称、product_key（全局唯一、系统生成）、描述、设备类型（传感器/执行器/复合）。  
验收：创建成功后 product_key 全局唯一，同一 product_key 不可重复创建。

**FR-P02 定义物模型**  
每个产品可定义属性列表，描述设备上报数据的结构。  
字段：属性名、数据类型（int/float/bool/string）、单位、最小值、最大值。  
验收：设备上报数据时，平台校验 payload 中的字段类型和范围，不合规的数据拒绝入库并记录异常日志。

**FR-P03 查询产品列表**  
支持分页查询产品列表，返回产品基本信息和在线设备数量。

---

### 4.2 设备管理

**FR-D01 注册设备**  
管理员可在指定产品下注册设备。  
字段：设备名称、device_id（全局唯一，可自定义或系统生成）、描述。  
系统自动生成 device_secret（32 位随机字符串）。  
验收：注册成功后返回 device_id 和 device_secret，device_secret 仅显示一次。

**FR-D02 设备认证接入**  
设备使用 device_id 作为 MQTT client_id 和 username，device_secret 作为 password 连接 EMQX。  
EMQX 通过 HTTP Auth 回调平台接口 `POST /internal/emqx/auth` 进行认证。  
平台校验通过后返回该设备的 ACL 权限列表（只允许操作自身 topic）。  
验收：正确密钥的设备能连接成功；错误密钥被拒绝，EMQX 返回 CONNACK reason code 0x86。

**FR-D03 设备在线状态追踪**  
EMQX 在设备上线/下线时回调 `POST /internal/emqx/webhook`，平台更新 Redis 中的在线状态（key: `device:online:{device_id}`，TTL 随设备下线清除）。  
验收：设备连接后 2 秒内 `GET /api/devices/:id` 返回 `status: online`；设备断开后 5 秒内状态变为 `offline`。

**FR-D04 查询设备列表**  
支持按产品、在线状态过滤，分页返回设备列表及最后上线时间。

**FR-D05 删除设备**  
软删除，设备记录保留，device_id 不可再用于认证。  
验收：删除后设备使用原密钥连接被拒绝。

---

### 4.3 数据采集

**FR-T01 遥测数据上报**  
设备发布 MQTT 消息到 `devices/{product_key}/{device_id}/telemetry`，QoS 1，payload 为 JSON。  
EMQX 规则引擎配置数据桥接，直接将数据写入 TDengine，无需经过 Go 服务。  
验收：设备上报数据后 100ms 内数据写入 TDengine；Go 服务通过订阅接收同一条消息做规则匹配，两条路径并行不互斥。

**FR-T02 遥测数据查询**  
`GET /api/devices/:id/telemetry` 支持参数：metric（属性名）、from（Unix 时间戳）、to、interval（聚合粒度：raw/1m/5m/1h）、limit（最多 10000 条）。  
验收：查询 raw 数据返回原始记录；指定 interval 时返回该时间粒度的平均值聚合。

**FR-T03 设备实时快照**  
`GET /api/devices/:id/snapshot` 返回该设备最近一条每个属性的上报值及时间戳。  
验收：响应时间 < 50ms（从 Redis 缓存读取，TDengine 作兜底）。

---

### 4.4 规则与告警

**FR-R01 创建规则**  
管理员可创建规则，规则绑定到某个产品（对该产品下所有设备生效）。  
条件字段：属性名、比较运算符（>、<、>=、<=、==、!=）、阈值、持续时间（秒，0 表示立即触发）。  
动作类型（至少支持两种）：生成告警记录、向设备下发指令。  
验收：规则创建后立即对新到数据生效，无需重启服务。

**FR-R02 持续时间窗口**  
当规则配置了持续时间 > 0 时，只有条件连续满足超过该时长才触发动作。  
验收：配置「温度 > 40°C 持续 60 秒」，模拟器持续上报 41°C，60 秒后触发告警；中途降温再升温，计时重置。

**FR-R03 告警查询**  
`GET /api/alarms` 支持按设备、产品、状态（active/resolved）、时间范围过滤，分页返回。  
告警包含：触发时间、设备、触发规则、触发值、当前状态。

**FR-R04 告警确认**  
运维人员可通过 `PUT /api/alarms/:id/resolve` 手动解除告警，需填写处理备注。

---

### 4.5 设备影子

**FR-S01 设置期望状态**  
`PUT /api/devices/:id/shadow/desired` 设置设备期望状态（如 `{"targetTemp": 26}`）。  
平台将 desired 状态发布到 `devices/{pk}/{id}/shadow/desired`，同时存储到 Redis 和 PostgreSQL。  
验收：设备在线时，发布 desired 后设备侧 2 秒内收到消息；设备离线时，desired 暂存，设备上线后自动补发。

**FR-S02 设备上报实际状态**  
设备发布到 `devices/{pk}/{id}/shadow/reported`，平台更新 reported 状态并计算 delta（desired 与 reported 的差值）。  
验收：`GET /api/devices/:id/shadow` 返回 desired、reported、delta 三个字段。

**FR-S03 上线自动同步**  
设备连接时（EMQX webhook 触发），若存在未同步的 desired 状态（delta 非空），平台自动重新下发。  
验收：设备先离线，期间修改 desired，设备重新上线后 5 秒内收到 desired 消息。

---

### 4.6 指令下发

**FR-C01 发送指令**  
`POST /api/devices/:id/commands` 下发指令，payload 包含 method 和 params。  
平台生成唯一 command_id，记录状态为 pending，发布到 `devices/{pk}/{id}/command`，立即返回 202 和 command_id。  
验收：接口响应时间 < 100ms，不阻塞等待设备响应。

**FR-C02 指令状态追踪**  
设备执行后发布响应到 `devices/{pk}/{id}/command/reply`，payload 包含 command_id、code（0 为成功）、message。  
平台更新指令状态为 success 或 failed。  
`GET /api/devices/:id/commands/:command_id` 查询指令执行状态。  
验收：设备回复后 1 秒内状态更新可查；超过 30 秒无回复自动标记为 timeout。

---

### 4.7 OTA 固件升级

**FR-O01 固件管理**  
管理员可上传固件文件（模拟：仅上传元数据，文件存 MinIO 或本地 volume）。  
字段：产品、版本号（SemVer）、MD5 校验和、变更说明。  
验收：同一产品同一版本号不可重复上传。

**FR-O02 创建升级任务**  
管理员选择目标产品、目标固件版本、目标设备范围（全部/指定设备列表），创建升级任务。  
平台向目标设备发布升级通知到 `devices/{pk}/{id}/ota`，包含下载 URL、目标版本、MD5。  
验收：任务创建后，在线设备立即收到通知；离线设备上线后收到通知。

**FR-O03 升级进度追踪**  
设备上报升级进度到 `devices/{pk}/{id}/event`，event_type 为 `ota_progress`，包含阶段（downloading/installing/success/failed）和进度百分比。  
`GET /api/ota/tasks/:id` 返回任务整体进度（各阶段设备数量统计）。

---

### 4.8 设备模拟器

**FR-SIM01 基础模拟设备**  
模拟器支持命令行参数指定：产品、设备数量、上报间隔（秒）、数据波动范围。  
内置四种设备类型：
- 温湿度传感器：周期上报 temperature（15-45°C）、humidity（30-90%）
- 烟感探测器：周期上报 smoke_level（0-100），可配置触发阈值
- 门禁控制器：接收 open/close 指令，上报 door_status
- 空调控制器：接收 setTemp 指令，上报 current_temp、target_temp、mode

**FR-SIM02 实现完整设备行为**  
模拟器实现：MQTT 连接与断线重连、遗嘱消息（`status: offline`）、接收并响应 command/shadow/ota 消息。

**FR-SIM03 压力测试模式**  
支持 `-stress -count=1000` 参数，启动 1000 个并发设备连接，每个设备每 5 秒上报一条数据。

---

## 5. 非功能需求

### 5.1 性能

| 指标 | 要求 |
|------|------|
| 并发设备连接数 | ≥ 1000（单机 Docker Compose） |
| 遥测消息吞吐 | ≥ 5000 msg/s |
| 数据入库延迟 | P99 < 100ms |
| API 响应时间 | P99 < 200ms（查询接口），P99 < 100ms（写入接口） |
| 快照接口响应 | P99 < 50ms |

### 5.2 可靠性

- 平台服务重启后，持久化会话保证设备消息不丢失（EMQX 持久会话 + QoS 1）
- 告警规则状态（滑动窗口）在进程重启后允许重置，不做持久化
- PostgreSQL 和 TDengine 数据通过 Docker volume 持久化

### 5.3 安全

- 设备使用 device_secret 做密码认证
- ACL 限制设备只能操作自身 topic
- EMQX 回调接口（`/internal/*`）只允许 EMQX 容器 IP 访问
- API 接口需要 JWT 认证（管理员和运维员不同权限）

### 5.4 可观测性

- 平台服务输出结构化日志（JSON），包含 trace_id、device_id、耗时
- 提供 `GET /metrics` Prometheus 格式指标（消息处理速率、规则触发次数、告警数量）
- 压测报告文件（包含吞吐量、延迟分布图）

---

## 6. 数据模型（核心表）

```sql
-- 产品
CREATE TABLE products (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(64) NOT NULL,
    product_key VARCHAR(32) UNIQUE NOT NULL,
    device_type VARCHAR(16) NOT NULL,  -- sensor | actuator | composite
    description TEXT,
    created_at  TIMESTAMPTZ DEFAULT now()
);

-- 物模型属性
CREATE TABLE product_properties (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  UUID REFERENCES products(id),
    name        VARCHAR(64) NOT NULL,
    data_type   VARCHAR(16) NOT NULL,  -- int | float | bool | string
    unit        VARCHAR(16),
    min_value   FLOAT,
    max_value   FLOAT,
    UNIQUE(product_id, name)
);

-- 设备
CREATE TABLE devices (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id     VARCHAR(64) UNIQUE NOT NULL,
    device_secret VARCHAR(64) NOT NULL,
    product_id    UUID REFERENCES products(id),
    name          VARCHAR(64),
    status        VARCHAR(16) DEFAULT 'inactive',  -- active | inactive | deleted
    last_online   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT now()
);

-- 规则
CREATE TABLE rules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id       UUID REFERENCES products(id),
    name             VARCHAR(64) NOT NULL,
    property_name    VARCHAR(64) NOT NULL,
    operator         VARCHAR(4) NOT NULL,   -- > < >= <= == !=
    threshold        FLOAT NOT NULL,
    duration_seconds INT DEFAULT 0,
    action_type      VARCHAR(16) NOT NULL,  -- alarm | command
    action_params    JSONB,
    enabled          BOOLEAN DEFAULT true,
    created_at       TIMESTAMPTZ DEFAULT now()
);

-- 告警
CREATE TABLE alarms (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id    VARCHAR(64) NOT NULL,
    rule_id      UUID REFERENCES rules(id),
    trigger_value FLOAT NOT NULL,
    status       VARCHAR(16) DEFAULT 'active',  -- active | resolved
    triggered_at TIMESTAMPTZ DEFAULT now(),
    resolved_at  TIMESTAMPTZ,
    resolve_note TEXT
);

-- 指令
CREATE TABLE commands (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id   VARCHAR(64) NOT NULL,
    method      VARCHAR(64) NOT NULL,
    params      JSONB,
    status      VARCHAR(16) DEFAULT 'pending',  -- pending | success | failed | timeout
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);

-- 设备影子
CREATE TABLE device_shadows (
    device_id VARCHAR(64) PRIMARY KEY,
    desired   JSONB DEFAULT '{}',
    reported  JSONB DEFAULT '{}',
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- OTA 固件
CREATE TABLE firmwares (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  UUID REFERENCES products(id),
    version     VARCHAR(32) NOT NULL,
    md5         VARCHAR(32) NOT NULL,
    file_url    TEXT NOT NULL,
    changelog   TEXT,
    created_at  TIMESTAMPTZ DEFAULT now(),
    UNIQUE(product_id, version)
);
```

---

## 7. MQTT Topic 规范

```
# 设备上行
devices/{product_key}/{device_id}/telemetry      QoS 1  设备周期上报遥测数据
devices/{product_key}/{device_id}/event           QoS 1  设备主动上报事件（告警、OTA进度）
devices/{product_key}/{device_id}/command/reply   QoS 1  设备响应平台指令
devices/{product_key}/{device_id}/shadow/reported QoS 1  设备上报实际状态

# 平台下行
devices/{product_key}/{device_id}/command         QoS 1  平台下发指令
devices/{product_key}/{device_id}/shadow/desired  QoS 1  平台下发期望状态
devices/{product_key}/{device_id}/ota             QoS 1  平台下发升级通知

# 平台订阅（共享订阅）
$share/platform/devices/+/+/telemetry            规则引擎消费
$share/platform/devices/+/+/event                事件处理
$share/platform/devices/+/+/command/reply        指令状态更新
$share/platform/devices/+/+/shadow/reported      影子同步
```

---

## 8. Payload 格式规范

**遥测上报**
```json
{
  "ts": 1722000000000,
  "values": {
    "temperature": 32.5,
    "humidity": 65.2
  }
}
```

**事件上报**
```json
{
  "ts": 1722000000000,
  "event_type": "ota_progress",
  "data": {
    "version": "1.2.0",
    "stage": "downloading",
    "progress": 45
  }
}
```

**指令下发**
```json
{
  "command_id": "uuid-xxx",
  "method": "setTemp",
  "params": { "target": 26, "mode": "cooling" }
}
```

**指令响应**
```json
{
  "command_id": "uuid-xxx",
  "code": 0,
  "message": "ok"
}
```

---

## 9. 里程碑与验收标准

### M1：设备接入（第 1 周末）
- [ ] Docker Compose 一键启动全部基础设施
- [ ] 模拟设备能通过认证连接 EMQX
- [ ] Dashboard 显示设备在线，平台 API 返回 `status: online`
- [ ] 错误密钥设备被拒绝

### M2：数据采集（第 2 周末）
- [ ] 模拟设备上报数据，TDengine 数据入库可查
- [ ] `GET /api/devices/:id/telemetry` 返回历史数据
- [ ] `GET /api/devices/:id/snapshot` 响应 < 50ms

### M3：设备影子与指令（第 3 周末）
- [ ] `PUT /api/devices/:id/shadow/desired` 在线设备 2 秒内收到
- [ ] 设备离线后修改 desired，重新上线后自动补发
- [ ] `POST /api/devices/:id/commands` 返回 202，30 秒超时自动标记

### M4：规则与告警（第 4 周末）
- [ ] 规则创建后对新数据立即生效
- [ ] 持续时间窗口逻辑验证（见 FR-R02 验收条件）
- [ ] 告警 CRUD 接口全通

### M5：OTA（第 5 周末）
- [ ] 固件上传、升级任务创建
- [ ] 在线设备收到 OTA 通知，模拟器上报 success
- [ ] 任务进度统计接口正确

### M6：前端面板（第 6 周末）
- [ ] 设备列表实时在线状态（MQTT.js WebSocket）
- [ ] 单设备历史数据曲线（ECharts）
- [ ] 告警列表与规则配置表单

### M7：压测与文档（第 7-8 周末）
- [ ] 1000 设备并发，5000 msg/s 吞吐，P99 延迟 < 100ms
- [ ] 压测报告文件
- [ ] README 含架构图、快速启动、演示截图
- [ ] Swagger API 文档可访问

---

## 10. 不在范围内（Out of Scope）

以下内容不在本项目交付范围，避免范围蔓延：

- 真实硬件接入（全部用模拟器代替）
- 多租户隔离
- mTLS 证书认证（只做用户名/密码）
- 移动端 App
- 邮件/短信告警推送（只做平台内告警记录）
- EMQX 集群部署（压测用单节点）

