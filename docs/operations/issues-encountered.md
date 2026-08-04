# 问题记录与排障总结

| 项目 | 内容 |
| --- | --- |
| 版本 | v1.0 |
| 日期 | 2026-08-02 |
| 状态 | Baseline |
| 关联文档 | [业务逻辑](../design/business-logic.md)、[架构](../design/architecture.md) |

本文记录开发与端到端验证过程中遇到的全部问题：现象、根因、解决方案与状态。
按"平台 → EMQX → 模拟器 → 环境"四类整理，供后续维护与复现参考。

---

## 一、平台侧

### 1. 生命周期 webhook 返回 500（SQLSTATE 42P08）

- **现象**：`POST /internal/emqx/webhook` 报 500，日志为 PostgreSQL
  `42P08: could not determine data type of parameter $2`
- **根因**：`SetDeviceStatus` 的 UPDATE 中 `$2` 同时被推断为 `varchar` 与 `text`
  （`SET status = $2` 与 `CASE WHEN $2 = 'online'`），PG 无法确定参数类型
- **解决**：显式转换 `SET status = $2::varchar`，`CASE WHEN $2::varchar = 'online'`，
  `$3::timestamptz`；`storage/metadata.go`
- **验证**：实机 webhook 链路 + 全量测试通过

### 2. 平台 MQTT 订阅在 broker 重启后静默丢失

- **现象**：重建 EMQX 容器后，平台连接正常但停止消费（消息计数器不再增长），
  设备全部"在线"却无遥测入库
- **根因**：paho 客户端 `clean_session=false` 时自动重连**不会重新订阅**——
  它假定 broker 会话仍存在；broker 重启清空会话后，平台处于"已连接但无订阅"状态
- **排查过程**：
  - 先尝试 `SetCleanSession(true) + SetResumeSubs(true)` → 无效
  - 读 paho v1.5.1 源码确认：`resume()` 只在 `CleanSession=false` 时执行；
    且 resume 只重发**断线时尚未完成 ACK** 的订阅包，长期订阅不恢复
- **解决**：`clean_session=false + ResumeSubs=true` 保留持久会话语义，同时在
  `Service.Start` 中监听 `Lost()` 通道，断线后显式重新执行 `subscribeAll`
  （paho 在未连接时会排队订阅请求，重连后自动发送）；`mqtt/service.go`
- **验证**：`--force-recreate emqx` 后消费速率恢复（9 msg/8s），共享订阅重新注册

### 3. 认证回调触发上线补发时设备尚未完成订阅

- **现象**：设备重连后收不到 OTA 通知与影子 desired（任务一直 pending）
- **根因**：认证回调在 CONNACK 后立即触发上线逻辑并发布消息，而设备此刻还没
  完成 SUBSCRIBE（clean session 无队列缓冲），通知丢失
- **解决**：认证路径的上线逻辑延迟 1 秒执行（`lifecycleGracePeriod`），
  给设备留出订阅时间；`httpapi/server.go`
- **验证**：离线 OTA 任务在设备重连后自动补发并 success

## 二、EMQX 侧

### 4. EMQX Webhooks 为 Enterprise 功能，OSS 不可用

- **现象**：`EMQX_WEBHOOKS__*` 环境变量静默无效；镜像内无 `emqx_webhook` 模块；
  dashboard API `/api/v5/webhooks` 404
- **根因**：EMQX 5.8（以及 5.7）Open Source 版已移除内置 Webhooks 特性，
  文档站点对应页面标题为 "EMQX Enterprise Docs"
- **解决**：改用**规则引擎 + HTTP 桥接**方案：`emqx-init` 一次性服务幂等创建
  HTTP 桥接 + `$events/client_connected` / `client_disconnected` 两条规则，
  转发到平台 `/internal/emqx/webhook`；`scripts/emqx-init.sh`

### 5. OSS 规则引擎收不到 client 事件（本环境）

- **现象**：规则创建成功（`enable=true`、actions 合法），但 `matched` 恒为 0；
  用内置 republish 动作测试也无效；在裸容器（无认证配置）中同样规则却能触发
- **根因**：本机 OrbStack 环境下的 EMQX 5.7/5.8 规则引擎事件管道不可用
  （事件未投递到规则引擎，具体机制未定位）
- **解决**：不依赖规则引擎的**替代链路**：
  - 上线：认证回调（每次连接必触发，已验证）→ 延迟 1s 执行上线逻辑
  - 下线：设备遗嘱消息（已验证投递）→ 平台消费 `{"status":"offline"}` 标记下线
  - 保留 webhook 端点与 `emqx-init` 桥接，供支持该能力的 EMQX 版本使用

### 6. EMQX 桥接/规则 API 使用要点（配置过程踩坑）

| 问题 | 说明 |
| --- | --- |
| 5.8 起 dashboard API 需登录换 token | `POST /api/v5/login` 拿 JWT，再用 `Authorization: Bearer` 访问管理 API |
| 桥接创建用扁平结构 | `POST /api/v5/bridges` 需 `type/http,url,method,body,headers` 平铺（`type` 会归一化为 `webhook`） |
| 详情 GET 需类型前缀 | 查桥接用 `bridges/webhook:<name>`、连接器用 `connectors/http:<name>` |
| 规则动作格式 | v2 桥接动作为 `{type}:{name}`（如 `webhook:iot_webhook`，API 归一化为 `http:iot_webhook`）；写成 `name:forward` 会被解析为不存在的 bridge_v1，规则被禁用 |
| 创建是集合 URL、检查是详情 URL | `POST /rules` 与 `GET /rules/<id>` 路径不同，脚本需区分 |

### 7. EMQX 5.7.2 镜像无 curl

- **现象**：`emqx-init` 卡在等待循环 120s 后失败（容器内 `curl: not found`）
- **解决**：`emqx-init` 改用 `curlimages/curl` 镜像（仅需 curl 客户端能力）

### 8. 本地 EMQX 镜像版本混淆（误判）

- **现象**：一度怀疑本地 `emqx/emqx:5.8.8` 是损坏镜像——lib 目录应用版本混杂
  （emqx-5.5.4、emqx_management-5.3.8、emqx_dashboard-5.2.2），且官方 amd64 镜像
  在 ARM 机器上启动崩溃（quicer NIF `nif_library_not_loaded`）
- **结论**：应用版本混杂是 EMQX 正常现象（应用独立版本号）；amd64 崩溃是
  **架构不匹配**（本机为 ARM，拉取 amd64 镜像运行会崩），与 iot-perform 栈无关；
  本机栈使用 arm64 镜像运行正常

## 三、设备模拟器侧

### 9. 遗嘱消息被误判为"旧连接"而忽略

- **现象**：SIGKILL 设备后状态不转 offline（保持 online）
- **根因**：遗嘱时间戳在连接建立前生成（`ConnectionOptions`），比认证时间
  （last_online）早约 18ms；平台按 `will.ts < last_online` 判定为旧连接遗嘱并忽略
- **解决**：比较加 5 秒容差——只有早于 `last_online - 5s` 的遗嘱才视为陈旧；
  `mqtt/service.go` + `TestProcessStaleWillIsIgnored`

### 10. 离线检测延迟约 60 秒（keepalive 默认 30s）

- **现象**：SIGKILL 设备后约 60s 才转 offline
- **根因**：MQTT keepalive 默认 30s，EMQX 需约 1.5× keepalive 才判定死连接并发布遗嘱
- **解决**：模拟器 keepalive 设为 3 秒，SIGKILL 后 1 秒内检测并转 offline
  （满足 SRS"断开后 5 秒内"验收）

### 11. paho 会话恢复卡死（不再发送 PUBACK）

- **现象**：设备重连后整机静默——不发布遥测、不回复指令；EMQX 侧 `inflight_cnt`
  持续堆积（send_msg.qos1 不回 ACK）
- **根因**：`clean_session=false` 恢复带未 ACK QoS1 消息的旧会话时，paho v1.5.1
  停止发送 PUBACK，客户端消息路由被卡死；会话被轮番重连的重复进程反复抢占加剧
- **解决**：模拟器改用 **clean session**（平台侧已有"上线补发"机制承担离线交付，
  不需要恢复 broker 会话）；`devicesim/paho.go`

### 12. 重复模拟器进程导致会话抢占（EOF 循环）

- **现象**：设备日志反复 `connection lost ... error=EOF`，EMQX 侧客户端时连时断
- **根因**：多代 `go run ./cmd/devicesim` 进程残留（go run 包装进程与编译产物子进程
  并存），多个同 clientid 客户端互相抢占会话
- **解决**：`pkill -f devicesim` 全清后按单个编译产物（`go build -o /tmp/devicesim`）
  启动；设备 ID 在凭据文件里而非命令行，`pkill` 匹配命令行无法命中，需按进程全量清理

## 四、环境与工具

### 13. 宿主机端口冲突（另一项目 EMQX 占用 1883/8083/18083）

- **现象**：iot-perform 的 EMQX 无法绑定标准端口
- **解决**：本机用 `docker-compose.override.yml` 映射到 2883/28083/28084
  （本地文件，不入库）；确认另一项目（mosquitto-starter）EMQX 停止后，
  已切回标准端口并验证
- **注意**：compose override 的 `ports` 列表是**合并**而非替换，需 `!override` 标签

### 14. 遥测查询 limit 返回的是最老样本

- **现象**：`?limit=3` 返回的是首次上报的数据而非最新数据
- **根因**：查询按 `ORDER BY ts ASC` 排序后截取 limit，取的是最早 N 条
- **处理**：查看最新数据需带 `from` 参数；行为已写入业务逻辑文档

### 15. MQTT 探测脚本协议错误（误判宿主机网络问题）

- **现象**：python 手写 MQTT CONNECT 包连接后无响应/EOF，一度怀疑宿主机端口转发故障
- **根因**：CONNECT 包 payload 顺序错误——MQTT 3.1.1 要求
  `clientid → will topic → will payload → username → password`，
  且 connect flags 必须带 username/password 标志位（0xC2）
- **处理**：改用 paho 客户端探测验证宿主机路径正常

### 16. 模拟器 tick 同步导致遥测 ts 相同，TDengine 主键覆盖丢数据（已修复）

- **现象**：1000 台压测时 TDengine 落库率仅 ~17%，10 台验证时仅 ~35%；
  平台 mqtt 计数与 EMQX received 均正常（零错误），数据在存储层消失
- **根因**：两层叠加——① 模拟器所有设备 `time.NewTicker` 同时启动，
  tick 相位完全同步，同一时刻发布的 payload `ts` 相同（实测 10 台设备
  每 tick 只产生 4 个不同毫秒）；② TDengine 表主键为 `ts` 单列，
  同 ts 的 INSERT 静默覆盖（不报错），平台无感知
- **处理**：模拟器连接后对 ticker 施加 0–1000ms 随机相位偏移
  （`internal/devicesim/device.go`），使各设备上报时间戳自然散布；
  修复后 10 台落库率 35% → 95%，1000 台落库率 ~75%
- **遗留（2026-08-04 已解决）**：TDengine 存储层由单普通表迁移为超级表/每设备子表
  （每设备独立 ts 主键，同毫秒多设备不再互相覆盖），迁移见
  [tdengine-stable-migration.md](../design/tdengine-stable-migration.md) 与
  `scripts/tdengine-migrate.sh`

### 17. 1000 台同时连接时认证回调瞬时超时（启动风暴）

- **现象**：模拟器启动瞬间约 10 台设备 `connect ... MQTT operation timed out`
- **根因**：1000 个并发连接同时触发 EMQX → 平台 `/internal/emqx/auth`
  HTTP 回调，瞬时打满平台认证处理（每次回调含设备查询）
- **处理**：模拟器按退避策略重连后全部上线（在线 1001 台稳定）；
  未改代码。可选优化：模拟器连接分批（并发上限）、平台认证加缓存

## 五、遗留观察项（非阻塞）

> 可落地的修复任务统一维护在 [TODO.md](../TODO.md)，
> 需求基线见 [architecture-evolution-srs.md](../requirements/architecture-evolution-srs.md)。

| 项目 | 状态 |
| --- | --- |
| 遥测写入曾出现 1 次 TDengine 超时（chaos 测试期间），此后 0 错误 | 观察 |
| 前端构建存在 chunk >500kB 警告 → `ARC-P1-13` | 非阻塞 |
| `mosquitto-starter` 的 EMQX 容器已停止（`docker start emqx` 可恢复） | 环境状态 |
