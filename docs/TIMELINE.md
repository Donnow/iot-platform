# 项目时间线（开发历程总览）

按时间顺序记录项目从零到当前的每个阶段：做了什么、产出什么、留下什么文档。
面试前扫这一篇，就能按时间顺序讲出完整故事线。

| # | 时间 | 阶段 | 核心内容 | 产出文档 |
| --- | --- | --- | --- | --- |
| 01 | 08-01 04:53–05:34 | 协议与骨架 | 设备模拟器测试合同、后端脚手架、依赖接口定义 | [device-simulator-test-plan.md](testing/device-simulator-test-plan.md) |
| 02 | 08-01 05:47–06:03 | 后端起步 | 内存仓储、HTTP API、MQTT 客户端集成、运行指标 | — |
| 03 | 08-01 08:47–11:18 | 功能铺开 | 可运行后端（认证/遥测/影子/OTA 仓储接入） | — |
| 04 | 08-01 19:35–20:44 | 全链路打通 | Vue 控制台、遥测/规则契约、OTA 后端、持久化存储（PG/Redis/TDengine）、API 契约 | [backend-api.md](api/backend-api.md)、[openapi.yaml](api/openapi.yaml) |
| 05 | 08-01 20:58–23:18 | 设备接入安全 | EMQX 认证/授权接入、TDengine 健康检查修复 | [architecture.md](design/architecture.md) |
| 06 | 08-02 01:20–01:43 | 验证与基线 | 生命周期/影子/OTA 验证流、broker 重启订阅恢复修复、业务逻辑与排障文档基线 | [business-logic.md](design/business-logic.md)、[issues-encountered.md](operations/issues-encountered.md)（1–15 条） |
| 07 | 08-03 03:28–03:33 | 上线准备 | 卫生检查（凭据入库防护）、根 README、GitHub 仓库上线 | [README.md](../README.md)、[make-jwt.sh](../scripts/make-jwt.sh) |
| 08 | 08-03 04:10–04:13 | 压测排障 | 1000 台压测 → 三方差分定位 → 发现模拟器 tick 同步导致 TDengine ts 覆盖 → 修复（相位偏移 35%→95%）→ 复盘 | [load-test.md](operations/load-test.md)、[load-test-debugging.md](operations/load-test-debugging.md)、issues 16–17 |
| 09 | 08-03 04:27–04:28 | 登录端点 | users 表 + bcrypt + JWT 签发、PG 仓储接线踩坑（501）、前端登录表单、三层验证链 → 复盘 | [login-endpoint-development.md](operations/login-endpoint-development.md) |
| 10 | 08-03 04:32–04:35 | CI 流水线 | GitHub Actions 三层设计（L1 测试 / L2 构建 / L3 e2e 后置）、测试依赖审计、首次运行全绿 → 复盘 | [ci-pipeline-development.md](operations/ci-pipeline-development.md) |
| 11 | 08-04 14:45–14:52 | 存储层改造 | TDengine 单普通表 → 超级表/每设备子表（子表名 md5 编码、DESCRIBE 校验、INSERT USING TAGS、查询改子表 + 应用层回填），迁移脚本 + 单测 + 文档同步 | [tdengine-stable-migration.md](design/tdengine-stable-migration.md)、[tdengine-migrate.sh](../scripts/tdengine-migrate.sh) |
| 12 | 08-05 00:15–00:35 | 消费流水线 | MQTT 单线程串行 → 分片 worker 池 + 产品缓存 + TDengine 批量写入（重试/隔离重放），含持久化规范与踩坑记录 | [persistence-spec.md](design/persistence-spec.md) |
| 13 | 08-05 | 架构演进规划 | 汇总分散遗留项，建立 P0/P1/P2 统一 TODO 与可靠性、安全、可观测性、扩容需求基线 | [TODO.md](TODO.md)、[architecture-evolution-srs.md](requirements/architecture-evolution-srs.md) |

## 每阶段一句话（面试叙事版）

1. **08-01 凌晨**：先定义"怎么算对"（模拟器测试合同）再写代码，接口先行
2. **08-01 白天**：内存模式跑通全链路，再换持久化存储——先快后稳
3. **08-01 晚**：EMQX 一机一密认证 + 设备级 ACL，接入层与业务解耦
4. **08-02**：验证全流程 + 修 broker 重启订阅丢失，15 条排障入库
5. **08-03 凌晨**：卫生检查后上线 GitHub，README 门面完成
6. **08-03 压测**：真实负载暴露"静默数据覆盖"——测量工具先行、三方差分定位、模拟器负载真实化
7. **08-03 登录**：认证闭环（bcrypt + JWT + 防枚举），接线错误靠端到端验证兜底
8. **08-03 CI**：分层流水线 + 测试依赖审计，徽章上 README
9. **08-04 存储**：压测暴露的"静默覆盖"架构债闭环——单表改超级表/每设备子表，配合幂等迁移脚本
10. **08-05 流水线**：消费端单线程瓶颈闭环——分片 worker + 产品缓存 + 批量写，写失败可重放
11. **08-05 演进规划**：不急于拆微服务，先用持久化消息边界、Outbox 和安全治理补齐生产基线

## 后续路线（未完成阶段）

后续状态统一见 [TODO.md](TODO.md)，验收口径见
[平台架构演进需求规格说明书](requirements/architecture-evolution-srs.md)。时间线只记录已经发生的
阶段，避免与任务清单产生重复状态。
