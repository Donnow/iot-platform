# 登录端点开发复盘：认证设计、接线踩坑与端到端验证

| 项目 | 内容 |
| --- | --- |
| 时间线编号 | #09（见 [TIMELINE.md](../TIMELINE.md)） |
| 版本 | v1.0 |
| 日期 | 2026-08-03 |
| 状态 | Baseline |
| 关联 | [后端接口与消息契约](../api/backend-api.md)、[问题记录与排障总结](issues-encountered.md) |

本文记录登录端点（`POST /api/auth/login`）从设计到落地的全过程：认证方案的
设计决策、两个真实踩坑（仓储接线遗漏、存量数据库迁移）、以及"单测 → curl →
浏览器"三层验证链。与压测排障复盘（load-test-debugging.md）互补：那篇讲
"怎么排查存量问题"，这篇讲"怎么设计新功能并避免同类坑"。

---

## 1. 背景与目标

### 1.1 为什么做

改造前平台的认证状态是"半成品"：

- 所有 `/api/*` 需要 Bearer JWT（HS256，`IOT_JWT_SECRET` 签名）
- 但**没有任何方式获取 token**——只能本地跑 `scripts/make-jwt.sh` 手搓
- 前端控制台有 token 弹窗，但要求用户**手动粘贴 JWT**，没有账号体系
- 已知限制第 7 条：JWT 无角色区分

面试场景下这是第一个会被问到的缺口："打开 Swagger，怎么登录？"

### 1.2 验收标准

1. `POST /api/auth/login {username, password}` 返回 JWT，token 可调通受保护 API
2. 密码安全存储（bcrypt），不落明文
3. 凭据错误与用户不存在响应一致（防用户名枚举）
4. 不破坏现有 8 处 `NewServer(...)` 测试调用
5. 存量 PostgreSQL volume（已有数据）升级后直接可用，无需重建容器
6. 前端控制台改为真正的登录表单

---

## 2. 整体思路：设计决策

### 2.1 认证链路

```
登录请求 ──> users 表查询（bcrypt 比对）──> IssueToken（HS256 + role claim）
                                                │
前端 localStorage 保存 token ──> 每次请求带 Authorization 头
```

| 决策点 | 选择 | 理由 / 反着选 |
| --- | --- | --- |
| 密码哈希 | bcrypt（golang.org/x/crypto） | 标准库无内置；bcrypt 自带盐 + 自适应代价。设备密钥仍为明文（已知限制），管理员密码必须先立标杆 |
| 账号存储 | PostgreSQL `users` 表 + memory 模式同步实现 | 与现有仓储模式一致；面试可讲"账号体系是平台功能不是配置项" |
| 初始管理员 | 启动时 bootstrap（`IOT_ADMIN_USERNAME` / `IOT_ADMIN_PASSWORD`，已存在则跳过） | 开箱即用 + 部署可覆盖；默认密码 `admin123456` 必须显著标注 |
| 错误响应 | 用户不存在 / 密码错误 → 同一 401 文本 | 防用户名枚举（面试高频安全点） |
| Token 有效期 | `IOT_JWT_TTL_SECONDS` 可配置，默认 3600 | 把"过期"变成可讲的设计点而非写死的数字 |
| 角色 | JWT 携带 `role` claim，接口层暂不校验 | 为分级留口子；避免一次做太多 |
| API 形状 | `{token, expires_in, role, username}` | 前端可直接消费，不用解 JWT |

### 2.2 兼容性策略：不改构造函数签名

现有测试 8 处调用 `NewServer(...)` / `NewServerWithOptions(...)`。若给构造函数
加 JWT 参数，全部要改。选择：`Server` 加两个**导出字段**（`JWTSecret`、
`JWTTTL`），main.go 装配时赋值；字段未设置时登录端点返回 `501`。

- 好处：零破坏现有测试；"未配置"有明确语义（501 而非 500/404）
- 代价：字段可被外部直接赋值——单体应用内部可接受，接口上不如构造函数严格
- 这是"演进存量代码"的通用手法：**加可选能力用字段/选项，不加必选参数**

### 2.3 存量数据库迁移：幂等建表

`migrations/001_init.sql` 通过 `docker-entrypoint-initdb.d` 只在 **PostgreSQL
volume 首次初始化**时执行。已运行的部署（本项目 postgres 容器已跑 30+ 小时）
不会自动获得 `users` 表——直接查表会报错。

方案：平台启动时执行 `CREATE TABLE IF NOT EXISTS users (...)`（幂等），
新老环境都覆盖。这是"迁移脚本只管新环境、运行时 ensure schema 兜底存量"
的双路径模式。

### 2.4 JWT 签发与验证对称

`IssueToken()` 与 `make-jwt.sh` 实现同一 scheme（HS256 + base64url），
验证方 `JWTAuthorizer` 只认算法和签名，不关心签发方——压测脚本的
`IOT_API_TOKEN` 方式继续可用，两条路径并存。

---

## 3. 踩坑记录

### 坑 1：PG 仓储接线遗漏——单测全绿，真实环境 501

**现象**：部署新镜像后 `POST /api/auth/login` 返回
`501 {"error":"Not Implemented","message":"user repository is not configured"}`，
bootstrap 日志也没有输出。

**排查**：501 文案直指 `s.repos.Users == nil`。单测（用 memory store）全绿，
因为 memory 的 `Repositories()` 注册了 `Users: s`；**但 PG 的
`storage.Store.Repositories()` 方法漏了 `Users: s`**——新写的仓储实现
（users.go）和接线点（Repositories()）是两个文件，漏了任何一环编译都不报错
（`Users` 是接口字段，nil 合法）。

**修复**：`storage/store.go` 的 `Repositories()` 补一行 `Users: s`。

**教训**：
- **单测绿 ≠ 能跑**：单测覆盖的是"仓储实现"，接线错误（谁注册了什么）
  只有真实装配路径（main.go → storage.New → Repositories()）才能暴露
- 新增仓储接口时，检查点应该是"接口 + PG 实现 + memory 实现 +
  **两个 Repositories() 接线**"四处，而不是三处
- 端到端验证（curl/浏览器）是唯一能兜住这类错误的环节——所以流程里
  单测之后必须走真实部署验证

### 坑 2：patch 工具吞换行（编辑事故）

两次 patch 操作把 `func run(...) error {` 和下一行合并成一行（old_string
末尾换行处理问题），`go vet` 立即报语法错误，当场修复。这是工具使用层的
坑，与代码无关，但提醒：**每次 patch 后跑 build/vet 是必须的**——本例两次
都被 lint 即时捕获，没有进入测试环节。

### 坑 3：OpenAPI 双份契约

平台代码内嵌一份 openapi.yaml（`internal/platform/httpapi/`），docs 下还有
一份，README 要求同步。改 API 时容易只改一份。本次手动 `cp` 同步并
`grep -c` 校验。**这种双份文件是架构债**，后续可考虑构建时从单一来源生成。

---

## 4. 验证链（三层）

| 层 | 方式 | 覆盖 |
| --- | --- | --- |
| 单测 | `TestLogin*` / `TestIssueToken*` / `TestHashPassword*`（6 个） | 签发-验证对称、过期拒绝、成功登录 + token 可用、错误密码与未知用户响应一致、未配置 501、短密码拒绝 |
| 真实部署 | 重建镜像 → bootstrap 日志 `bootstrapped admin user` → curl 登录 → token 调 `/api/products` 200 → 错误密码 401 | 接线、迁移、配置装配 |
| 浏览器 | 打开控制台 → 401 自动弹登录 → 输入 admin/admin123456 → 数据面板加载 | 前端登录表单、localStorage 持久化、401 触发登录 |

其中第 2 层当场抓到了坑 1（接线遗漏）——三层缺一不可。

---

## 5. 方法论沉淀（面试可讲）

1. **演进存量代码：加能力不加参数**——新功能用导出字段/选项接入，构造函数
   签名不动，零破坏现有调用；未配置时有明确语义（501）
2. **新增仓储接口的检查点是四处**：接口定义、PG 实现、memory 实现、
   **两个 Repositories() 接线**——接线错误编译不报错，只有真实装配路径暴露
3. **单测绿 ≠ 能跑**：接线、配置、迁移这类"装配层"问题必须靠真实部署的
   端到端验证兜底（curl + 浏览器）
4. **迁移的双路径模式**：初始化脚本（新环境）+ 运行时幂等 ensure schema
   （存量环境）——升级不重建，是生产环境的基本素养
5. **安全细节三件套**：bcrypt 哈希（不存明文）、统一 401（防用户名枚举）、
   默认密码强制提示修改（`IOT_ADMIN_PASSWORD` 部署必改）
6. **同一个 scheme 两个签发方**：`IssueToken` 与 `make-jwt.sh` 并存——
   验证方只认算法与签名，工具链（压测）与产品功能互不阻塞

---

## 6. 遗留与后续

| 项 | 说明 |
| --- | --- |
| 角色分级 | JWT 已有 `role` claim，接口层未校验；SRS 的管理员/运维分级待做 |
| 登录限流 | 登录端点暂无防爆破（速率限制/锁定）；生产部署应加 |
| 双份 openapi | docs 与内嵌契约手工同步，建议构建期单一来源生成 |
| 设备密钥哈希 | 管理员密码已 bcrypt，设备密钥仍明文（已知限制第 5 条） |
