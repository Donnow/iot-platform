# 文档目录

本目录集中维护智慧园区 IoT 平台的需求、设计、接口、测试和运维文档。文档以 Markdown 为主，代码中的协议、接口和部署配置发生变化时，应同步更新对应文档。

## 目录结构

```text
docs/
├── README.md                         # 文档入口和维护规范
├── requirements/                     # 产品需求和验收基线
│   └── iot-platform-srs.md
├── design/                           # 架构和关键技术设计
│   ├── architecture.md
│   └── business-logic.md
├── api/                              # HTTP、MQTT 和内部接口契约
│   ├── backend-api.md
│   └── openapi.yaml
├── testing/                          # 测试计划、测试合同和验收记录
│   ├── backend-test-plan.md
│   └── device-simulator-test-plan.md
└── operations/                       # 部署、配置、运行手册和压测报告
    ├── device-simulator.md
    ├── platform.md
    ├── issues-encountered.md
    └── load-test.md
```

当前已建立需求基线和设备模拟器测试合同；其他目录在产生对应内容时再创建，避免提交空目录。

## 文档索引

> **看时间线先看 [TIMELINE.md](TIMELINE.md)**——按时间顺序记录每个阶段的
> 产出与文档链接，面试前扫一篇即可讲完整故事线。

| # | 时间 | 文档 | 用途 | 状态 |
| --- | --- | --- | --- | --- |
| — | — | [TIMELINE.md](TIMELINE.md) | 项目开发历程时间线总览 | Baseline |
| 01 | 08-01 | [IoT 平台需求规格说明书](requirements/iot-platform-srs.md) | 产品范围、功能需求、协议规范和里程碑基线 | Baseline |
| 02 | 08-01 | [设备模拟器测试合同](testing/device-simulator-test-plan.md) | 模拟器的行为、协议、联调和压力验收标准 | Draft |
| 03 | 08-01 | [后端测试计划](testing/backend-test-plan.md) | 后端单元、进程内集成和运行时校验 | Draft |
| 04 | 08-01 | [设备模拟器运行手册](operations/device-simulator.md) | 模拟器启动、凭据、压力模式和重连说明 | Draft |
| 05 | 08-01 | [后端接口与消息契约](api/backend-api.md) | HTTP、MQTT、EMQX 内部回调和当前实现边界 | Draft |
| 06 | 08-01 | [OpenAPI 契约](api/openapi.yaml) | 可导入 Swagger UI 的 HTTP 接口定义 | Review |
| 07 | 08-01 | [后端平台运行手册](operations/platform.md) | Compose 启动、健康检查、指标和排障 | Draft |
| 08 | 08-01 | [平台架构](design/architecture.md) | 运行拓扑、数据职责和启动模式 | Draft |
| 09 | 08-02 | [平台业务逻辑详解](design/business-logic.md) | 全部业务流程、状态机、消息契约与容错机制 | Baseline |
| 10 | 08-02 | [问题记录与排障总结](operations/issues-encountered.md) | 开发与验证中遇到的问题：现象、根因、解决方案（17 条） | Baseline |
| 11 | 08-03 | [压力测试运行记录](operations/load-test.md) | 可复现的 1000 设备压测命令和结果记录 | Draft |
| 12 | 08-03 | [压测排障复盘](operations/load-test-debugging.md) | 压测方法论、排障过程、Bug 根因与修复思路（面试素材） | Baseline |
| 13 | 08-03 | [登录端点开发复盘](operations/login-endpoint-development.md) | 认证设计决策、接线踩坑、三层验证链（面试素材） | Baseline |
| 14 | 08-03 | [CI 流水线搭建复盘](operations/ci-pipeline-development.md) | CI 分层设计、测试依赖审计、首次运行结果与 L3 路线 | Baseline |

运行中的 Swagger UI 地址为 `http://localhost:8080/docs`，原始契约地址为
`http://localhost:8080/openapi.yaml`。嵌入式契约位于
`internal/platform/httpapi/openapi.yaml`，修改 API 时需要同步更新
`api/openapi.yaml`。

## 命名和编排规范

- 文件名使用小写 kebab-case，名称应表达文档主题，例如 `device-simulator-test-plan.md`。
- 每份文档开头说明标题、版本、日期和状态；状态统一使用 `Draft`、`Review`、`Baseline`、`Deprecated`。
- 需求文档描述“系统必须做什么”；设计文档描述“系统如何实现”；测试文档描述“如何证明实现满足要求”。
- 需求、协议或接口发生变更时，先更新对应基线文档，再更新实现和测试。
- 测试文档中的测试编号保持稳定；删除测试时保留编号并标记为 `Deprecated`，避免验收记录失去对应关系。
- 文档内链接使用相对路径，提交后应能从 `docs/README.md` 逐级访问。
- 每个独立变更完成后先检查 Markdown 链接、格式和内容一致性，再提交 Git。

## 变更记录要求

涉及以下内容的变更需要在对应文档中留下可追踪信息：

- 产品需求、验收条件或范围变化：更新 `requirements/` 文档的版本和状态。
- MQTT topic、payload 或 HTTP API 变化：同步更新 `api/` 和相关测试文档。
- 行为或性能标准变化：同步更新 `testing/` 文档，并说明受影响的测试编号。
- 部署参数、环境变量或排障流程变化：更新 `operations/` 文档。
