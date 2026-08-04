# IoT 平台架构演进 TODO

| 项目 | 内容 |
| --- | --- |
| 版本 | v0.1 |
| 日期 | 2026-08-05 |
| 状态 | Active |
| 需求基线 | [平台架构演进需求规格说明书](docs/requirements/architecture-evolution-srs.md) |

本文是仓库中**未完成工作的唯一状态清单**。README、时间线、设计复盘中的
Roadmap 或“后续”章节只保留背景说明，任务状态统一在这里更新。

状态约定：`[ ]` 未开始，`[-]` 进行中，`[x]` 已完成，`[!]` 阻塞。完成任务时必须
同时补齐代码、测试和受影响文档，并在“完成记录”中写入日期与验证结果。

## P0：进入生产环境前必须完成

### [ ] ARC-P0-01 Redis 缓存故障降级

- 目标：落实“PostgreSQL 是权威存储，Redis 是缓存”的数据职责。
- 范围：设备影子、在线状态、启动依赖与缓存错误指标。
- 交付物：Redis fail-open 读写策略、在线状态降级语义、故障注入测试、运行手册。
- 验收：Redis 不可用时影子仍可从 PostgreSQL 读取；PostgreSQL 写成功后不因缓存回写失败向客户端报告业务失败；降级过程有日志和指标。
- 需求：`FR-REL-01`、`NFR-REL-02`。

### [ ] ARC-P0-02 EMQX 内部回调安全边界

- 目标：禁止外部请求伪造设备认证、ACL 或生命周期事件。
- 范围：`/internal/emqx/auth`、`/internal/emqx/acl`、`/internal/emqx/webhook`。
- 交付物：独立内部监听地址或网络隔离、请求签名或 mTLS、失败审计与限流。
- 验收：无有效内部凭据的请求返回 401/403；公开 API 监听面不能直接调用内部回调；EMQX 正常认证和生命周期链路不回归。
- 需求：`FR-SEC-01`、`NFR-SEC-01`。

### [ ] ARC-P0-03 管理台实时通道去除平台级 MQTT 凭据

- 目标：浏览器不再持有可订阅全局设备 topic 的长期 Broker 账号密码。
- 范围：前端 MQTT.js 直连、后端事件推送、JWT 鉴权与设备范围过滤。
- 交付物：SSE/WebSocket 实时接口或短期最小权限 MQTT 凭据；前端迁移和回归测试。
- 验收：生产构建产物中不含 `VITE_MQTT_USERNAME`/`VITE_MQTT_PASSWORD`；未授权用户无法订阅设备状态和事件；在线状态仍能实时刷新。
- 需求：`FR-SEC-03`、`FR-FE-02`。

### [ ] ARC-P0-04 设备密钥安全存储、轮换与吊销

- 目标：数据库不保存可直接用于设备登录的明文密钥。
- 范围：设备注册、EMQX 认证、密钥轮换、旧密钥吊销、日志脱敏。
- 交付物：适合高熵设备密钥的摘要方案、`key_version`、轮换 API、迁移脚本与兼容窗口。
- 验收：数据库泄露不能直接恢复设备登录凭据；新密钥仅在创建或轮换响应中显示一次；旧密钥在窗口结束后失效。
- 需求：`FR-SEC-02`、`NFR-SEC-02`。

### [ ] ARC-P0-05 遥测持久化接入与可重放死信

- 目标：平台确认接收的 QoS 1 消息在进程崩溃、队列满或 TDengine 暂时故障时仍可恢复。
- 范围：MQTT 接入队列、批写 pending、重试、毒消息隔离、人工重放。
- 交付物：持久化接入日志或流、幂等消费键、死信查询/重放能力、容量与保留策略。
- 验收：队列满时不静默丢弃已确认消息；消费者在写入前崩溃后可重放；重复投递不产生重复业务副作用；死信超过阈值触发告警。
- 需求：`FR-REL-02`、`FR-REL-04`、`NFR-REL-01`。

### [ ] ARC-P0-06 下行消息 Transactional Outbox

- 目标：命令、影子 desired、OTA 与规则动作的数据库状态和 MQTT 发布最终一致。
- 范围：HTTP 写操作、规则触发命令、后台发布器、重试和幂等。
- 交付物：`outbox_events` 迁移、事务内事件写入、dispatcher、重试/死信指标。
- 验收：业务状态与 outbox 事件在同一 PostgreSQL 事务提交；发布失败可自动重试；进程在提交后崩溃不会永久漏发；重复发布由业务键去重。
- 需求：`FR-REL-03`、`FR-REL-05`、`NFR-REL-03`。

## P1：可靠性基线完成后实施

### [ ] ARC-P1-01 提取 application/usecase 层

- 目标：HTTP 和 MQTT 只承担协议适配，业务编排集中在可测试的应用服务中。
- 首批用例：`SendCommand`、`UpdateDesiredShadow`、`CreateOTATask`、`HandleTelemetry`、`SetDeviceLifecycle`。
- 验收：协议层不直接编排多个 repository 与 publisher；用例层覆盖成功、重试、冲突和部分依赖故障测试。
- 需求：`FR-APP-01`。

### [ ] ARC-P1-02 规则缓存与 OTA 定向查询

- 目标：消除每条遥测查询规则和 OTA 进度扫描产品全部任务的放大效应。
- 验收：规则读取具备按产品缓存和显式失效；OTA 进度能由稳定业务键直接定位任务；压测中 PostgreSQL 查询量不再与消息数一比一增长。
- 需求：`FR-APP-02`、`FR-APP-07`、`NFR-PERF-02`。

### [ ] ARC-P1-03 产品/规则完整 CRUD 与告警自动恢复

- 范围：产品更新，规则更新/启停/删除，告警条件恢复后自动解除或进入 recovered 状态。
- 验收：修改规则后无需重启即可生效并使缓存失效；恢复事件不会重复解除人工已处理告警；API、OpenAPI、前端和测试同步更新。
- 需求：`FR-APP-03`、`FR-APP-04`。

### [ ] ARC-P1-04 指令后台超时处理

- 目标：不依赖查询请求触发 timeout 状态。
- 验收：pending 指令超过配置时限后由后台任务标记 timeout；任务可多实例安全执行；状态变化有指标和测试。
- 需求：`FR-APP-05`。

### [ ] ARC-P1-05 遥测快照缓存

- 目标：达到原 SRS 的快照 P99 小于 50ms 要求。
- 验收：遥测消费成功后更新快照；Redis 缺失时从 TDengine 回源并回填；Redis 故障时按 `ARC-P0-01` 降级；压测记录 P50/P95/P99。
- 需求：`FR-APP-06`、`NFR-PERF-03`。

### [ ] ARC-P1-06 设备影子版本与并发控制

- 目标：实现 README 曾声明但当前尚未落地的 version 乐观锁。
- 验收：影子响应包含单调递增 version；客户端可携带 expected_version 更新；版本冲突返回 409 且不会覆盖新状态；上线补发携带 version。
- 需求：`FR-APP-08`。

### [ ] ARC-P1-07 存活/就绪探针

- 目标：区分进程存活与对外服务能力。
- 验收：`/livez` 只反映进程；`/readyz` 反映 PostgreSQL、MQTT 订阅、遥测写入通道及必要依赖状态；Compose 使用 healthcheck 而非仅 `service_started`。
- 需求：`FR-OPS-01`。

### [ ] ARC-P1-08 可观测性补全

- 范围：HTTP 路由延迟/状态码、MQTT 队列深度、pending/dead-letter 数量、批写延迟、outbox 延迟、缓存降级、登录限流、trace/request ID。
- 验收：核心链路能通过 request_id/message_id/device_id 关联；指标具有 HELP/TYPE；关键失败有明确告警阈值和排障说明。
- 需求：`FR-OPS-02`、`FR-OPS-03`、`NFR-OBS-01`。

### [ ] ARC-P1-09 PostgreSQL/TDengine 版本化迁移

- 目标：替代“仅首次启动 SQL + 启动时零散 ensure schema”的不可追踪升级方式。
- 验收：新库和存量库执行同一迁移序列得到一致 schema；迁移记录可查询；失败停止启动且不留下半完成状态；TDengine 结构变更也有版本记录和回滚说明。
- 需求：`FR-OPS-04`。

### [ ] ARC-P1-10 CI L3 Compose E2E

- 范围：启动完整 Compose、注册设备、devicesim 上报、遥测查询、命令/影子/OTA 验证、latencyprobe、失败日志归档。
- 验收：支持 `workflow_dispatch` 和定时运行；失败时上传日志与关键指标；复用真实部署配置而非 memory store。
- 需求：`FR-OPS-06`。

### [ ] ARC-P1-11 RBAC 与登录防爆破

- 范围：admin/operator 权限矩阵、接口级授权、登录速率限制、统一审计。
- 验收：operator 无法执行凭据轮换、用户管理等管理员操作；连续失败登录受到限制；认证失败不泄露用户名是否存在。
- 需求：`FR-SEC-04`。

### [ ] ARC-P1-12 OpenAPI 单一事实来源与文档一致性

- 目标：消除 `docs/api/openapi.yaml` 与嵌入副本的手工同步，以及 SRS/README 与实现偏差。
- 验收：构建时只从一个 OpenAPI 源生成或嵌入；CI 校验生成结果无差异；完成一次需求、接口、README、运行手册一致性审计。
- 需求：`FR-OPS-05`、`NFR-MAINT-01`。

### [ ] ARC-P1-13 前端按业务域拆分并补测试

- 范围：overview/devices/alarms/rules/products/ota 视图组件、API composable、实时连接 composable、表单与状态测试，以及 ECharts/业务视图按需加载。
- 验收：`App.vue` 只保留应用壳与导航；各业务域可独立测试；不为当前规模强制引入全局状态框架；生产构建不再出现 500 kB chunk 警告并记录 bundle budget。
- 需求：`FR-FE-01`、`FR-FE-03`、`FR-FE-04`。

### [ ] ARC-P1-14 连接风暴保护

- 目标：1000 台设备同时重连时认证和生命周期链路不出现瞬时超时。
- 验收：模拟器支持连接抖动/分批；认证查询具备有界缓存或并发保护；1000 台重连测试中最终全部上线且认证端点错误率满足要求。
- 需求：`FR-REL-06`、`NFR-PERF-04`。

## P2：达到扩容门槛后再实施

### [ ] ARC-P2-01 同一代码库支持 API/ingestion 运行角色

- 目标：不拆微服务代码库，也能独立扩缩 HTTP API 与 MQTT 消费者。
- 验收：支持 `all`、`api`、`ingestion` 三种角色；角色缺失依赖时配置校验明确；Compose 覆盖默认 `all` 模式。
- 需求：`FR-SCALE-01`。

### [ ] ARC-P2-02 多实例设备保序与规则窗口外置

- 前置：确实需要多个 ingestion 实例。
- 验收：同设备消息在多实例场景保持所需顺序；规则持续时间状态重启不丢失或明确采用可接受的重置策略；故障转移有集成测试。
- 需求：`FR-SCALE-02`。

### [ ] ARC-P2-03 Kafka 技术决策与 5000 msg/s 验证

- 触发条件：持续负载超过当前流水线能力，或出现独立消费者、跨实例回放、长周期保留需求。
- 交付物：ADR、Redis Streams/WAL/Kafka 对比、容量估算、故障模型、运维成本和回滚方案。
- 验收：先用当前架构完成 5000 msg/s 基准；只有无法满足 SLO 或业务能力明确需要时才引入 Kafka；引入后验证端到端重放和消费者扩缩容。
- 需求：`FR-SCALE-03`、`NFR-PERF-05`。

### [ ] ARC-P2-04 遥测冷热分层与保留策略

- 前置：明确在线保留天数、归档周期、查询范围和成本目标。
- 验收：归档不阻塞热路径；历史查询语义明确；删除和恢复流程可验证。
- 需求：`FR-SCALE-04`。

### [ ] ARC-P2-05 镜像发布与依赖升级自动化

- 范围：tag 推送 GHCR、SBOM/镜像扫描、GitHub Actions 和基础镜像版本更新。
- 验收：`v*` tag 生成可追踪镜像；镜像关联 Git SHA；依赖更新通过测试、构建和 E2E 后才能合并。
- 需求：`FR-OPS-07`。

## 完成定义

每个任务完成前至少满足：

- [ ] 对应需求条目的验收条件全部通过
- [ ] `go test ./... -count=1` 与 `go vet ./...` 通过
- [ ] `cd frontend && npm test && npm run build` 通过（涉及前端或 API 时）
- [ ] 新增或更新单元测试、集成测试及故障路径测试
- [ ] 同步 OpenAPI、需求、设计、测试或运行手册中的受影响文档
- [ ] 记录配置变更、迁移步骤、兼容性和回滚方式
- [ ] 更新本文件状态与完成记录

## 完成记录

| 日期 | ID | 结果 | 验证 |
| --- | --- | --- | --- |
| 2026-08-03 | 登录端点（bcrypt 管理员密码 + JWT 签发） | 完成 | `go test ./...` 全绿 |
| 2026-08-03 | 1000 台设备/200 msg/s 单机压测与延迟探针 | 完成 | [load-test.md](docs/operations/load-test.md)、latencyprobe 记录 P50/P95/P99 |
| 2026-08-03 | 后端测试、前端测试和 Docker build CI 基础层 | 完成 | [ci.yml](.github/workflows/ci.yml) 三作业全绿 |
| 2026-08-04 | TDengine 超级表/每设备子表迁移 | 完成 | `go test ./...` 全绿；迁移脚本幂等/重跑/`--drop-legacy` 四路径验证 |
| 2026-08-05 | MQTT 分片 worker、产品缓存和 TDengine 批量写入 | 完成 | `go test ./... -race` 全绿；批量/缓存/重试/隔离重放单测 12 条 |

## 已完成基线（不再作为代办）

- [x] 登录端点、bcrypt 管理员密码和 JWT 签发
- [x] 1000 台设备/200 msg/s 单机压测与延迟探针
- [x] TDengine 超级表/每设备子表迁移
- [x] MQTT 分片 worker、产品缓存和 TDengine 批量写入
- [x] 后端测试、前端测试和 Docker build CI 基础层
