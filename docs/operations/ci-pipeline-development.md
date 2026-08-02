# CI 流水线搭建复盘：分层设计、测试依赖审计与首次运行

| 项目 | 内容 |
| --- | --- |
| 版本 | v1.0 |
| 日期 | 2026-08-03 |
| 状态 | Baseline |
| 关联 | [.github/workflows/ci.yml](../../.github/workflows/ci.yml)、[压测排障复盘](load-test-debugging.md) |

本文记录 GitHub Actions 流水线（L1 快层 + L2 构建层）的设计与落地：
分层理由、搭建前的测试依赖审计、首次运行结果与后续路线（L3 集成层）。

---

## 1. 目标与范围

### 1.1 为什么做

- README 顶部 CI 徽章是简历门面，招聘者第一眼可见
- 自动化质量门禁：vet + 全量单测 + 前端测试 + 镜像构建，每次 push 自动验证
- 面试话题："CI 为什么分层、为什么 L3 不阻塞主干"

### 1.2 范围（本次）

| 层 | Job | 内容 |
| --- | --- | --- |
| L1 快层 | Backend (vet + test) | `go vet ./...` + `go test ./... -count=1`（无外部依赖） |
| L1 快层 | Frontend (test + build) | `npm ci` + `npm test` + `npm run build` |
| L2 构建层 | Docker images (build check) | 构建 platform + frontend 镜像，验证 Dockerfile 不被破坏 |

L3（完整 compose e2e，复用 devicesim/latencyprobe）按用户决定后置，
用 `workflow_dispatch` 手动触发，不阻塞主干。

---

## 2. 搭建前审计：测试能否在干净环境跑绿

CI 最大的隐性风险是**测试静默依赖外部服务**（真 PG / EMQX / TDengine），
本地能过是因为 compose 一直在跑，换到 GitHub runner 必挂。搭建前逐项审计：

| 测试 | 依赖 | 结论 |
| --- | --- | --- |
| `internal/platform/integration_test.go` | 有 `//go:build integration` tag | **默认不编译**，`go test ./...` 不会执行；L3 再用 `-tags integration` 显式运行 |
| storage 包测试 | `roundTripFunc` mock HTTP 客户端（telemetry_test.go） | 不连真 TDengine ✓ |
| httpapi / mqtt / memory / devicesim | httptest + memory store + fake clock | 无外部依赖 ✓ |
| 前端 vitest | jsdom | 无浏览器依赖 ✓ |

结论：`go test ./...` 在干净 runner 可跑绿，无需在 CI 里起中间件。

**方法论沉淀**：新增测试时用 build tag 区分"单元/集成"（integration tag），
集成测试显式声明外部依赖——这是 CI 能分层的前提。压测复盘里提过
"单测绿 ≠ 能跑"，这里的补充是"能跑 ≠ 能在 CI 跑"——依赖审计要在
写 workflow 之前做，而不是等第一次红。

---

## 3. 设计决策

| 决策点 | 选择 | 理由 |
| --- | --- | --- |
| Go 版本 | `go-version-file: go.mod`（1.24） | 单一事实来源，不用手写版本号 |
| Go 缓存 | `actions/setup-go` 内置 cache | 零配置 |
| npm 缓存 | `actions/cache` 语义（setup-node `cache: npm` + lockfile 路径） | lockfile 作 key 自动失效 |
| 测试缓存 | `-count=1` 禁用 | CI 上禁用本地缓存语义，避免"上次的假绿" |
| 镜像构建 | `docker build` 只构建不推送 | 验证 Dockerfile 可构建；GHCR 推送策略后置（tag 时推） |
| 触发 | push main + PR | 主干与 PR 同门禁 |
| 密钥 | workflow 中不出现任何真实凭据 | .env 不入库（.gitignore 兜底），JWT secret 用默认测试值 |

已知小问题：`actions/checkout@v4` / `setup-node@v4` 触发 Node 20 deprecation
警告（runner 强制跑在 Node 24），不影响结果；后续升级 v5 消除。

---

## 4. 首次运行结果

2026-08-03 push `0354f56` 后自动触发，三个 job 并行，全部成功：

| Job | 结果 | 备注 |
| --- | --- | --- |
| Backend (vet + test) | ✓ | vet + 全量单测（含 login 端点 6 个新测试） |
| Frontend (test + build) | ✓ | vitest 2 个 + vite build |
| Docker images (build check) | ✓ | platform（distroless 多阶段）+ frontend（nginx） |

README 徽章 URL 已验证可访问（HTTP 200，GitHub 徽章有 5 分钟缓存）。

---

## 5. 方法论沉淀（面试可讲）

1. **CI 分层：快反馈与全量验证分离**——每次 push 只跑分钟级快层，
   重型 e2e 手动/定时触发。面试答"CI 怎么设计"的骨架
2. **搭建前做测试依赖审计**——列出每个测试包的外部依赖，
   集成测试用 build tag 隔离；本地绿 ≠ CI 绿，问题要在写 workflow 前发现
3. **单一事实来源**——go 版本读 go.mod、npm 缓存锁 lockfile，
   不复制版本号到 workflow
4. **`-count=1` 禁缓存**——CI 的测试结果必须来自本次代码，
   不能用缓存"假绿"（与压测复盘"测量工具先验证"同源：结果必须真实可复现）
5. **徽章即简历**——README 顶部 badge，面试官/招聘者第一眼

---

## 6. 后续路线（L3 及演进）

| 项 | 说明 |
| --- | --- |
| L3 e2e job | `workflow_dispatch` 手动触发：起完整 compose（EMQX/PG/Redis/TDengine）→ devicesim 模拟器 → 遥测查询断言 → 可选 latencyprobe；复用压测资产，写 `scripts/ci-e2e.sh` |
| GHCR 推送 | tag（v*）时推送镜像，配 Docker metadata action |
| Actions 版本升级 | checkout/setup-* 升 v5 消除 Node 20 警告 |
