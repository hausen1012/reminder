# CLAUDE.md

## 系统与语言

- 所有对话使用中文。
- 所有文档使用中文（含提交说明、计划、总结）。
- 所有代码注释使用中文，PowerShell 脚本除外。

## 项目

自托管提醒系统：Go + Gin + GORM + SQLite 后端，React + TypeScript + Vite 前端，
前端构建产物经 `go:embed` 打进二进制，部署为单个可执行文件 + 一个数据库文件。

```
backend/    cmd/server 入口，internal/ 下分 handlers / services / scheduler / notifier / middleware 等
frontend/   src/pages、components、contexts、lib/api.ts
docs-site/  对外文档站（VitePress），是对外唯一权威文档
```

常用命令：`make dev` / `make fmt` / `make lint` / `make test` / `make build` / `make docker`。
`backend/cmd/server/web/`、`frontend/dist/`、`build/`、`*.db`、`logs/` 均为生成物，不提交。

## 硬性要求

### 1. 代码改动要确认文档是否更新，但严禁过度描述

每次改完代码，主动判断是否需要同步文档，并在回复里明确说明「更新了哪些文档」或「无需更新及原因」。主要对照：
- 接口 / 字段 / 错误码变化 → `docs-site/`
- 环境变量变化 → `.env.example`、`docs-site/guide/configuration.md`
- 构建脚本变化 → `README.md`、`docs-site/guide/development.md`

只改内部实现、不影响对外行为的重构，无需更新文档，但要说明。

### 2. 文档与注释精简规范（拒绝废话与心路历程）

文档与代码注释只维护**关键必要内容**，严禁塞入啰嗦话、设计辩解与历史痕迹：

- **只陈述客观事实与规则，不写设计辩解**：
  文档只说明「功能是什么、如何使用、生效规则与边界限制」。严禁把设计讨论、心路历程写进文档（例如：“刻意不做 XX”、“为什么不提供这个设置项”、“避免用户误会”、“这样做是因为旧方案有缺陷”等自我辩护一律不写）。
- **禁止把变更历史当永久文档**：
  严禁在文档或代码注释中留下阶段性痕迹（如“Phase X 接入”、“设置项只剩一项了”、“我们没有新增环境变量”、“代码里不再内嵌 HTML”等属于 PR 说明的话术一律不得落库）。
- **严控 FAQ 准入门槛，禁止为改动强行写 FAQ**：
  - FAQ 只收录**真正的高频故障与排障步骤**（如端口连不上、签名不匹配、环境差异）。
  - 正常的参数限制（如“重发间隔最小 60 秒”）、架构定位（如“单进程单用户架构”），直接写在参数表格或架构说明中，**严禁专门开一条 FAQ 去讲道理或发牢骚**。
  - 禁止在多个页面分散写 FAQ 导致口径矛盾。
- **杜绝跨页面重复搬运**：
  概念和表格只在唯一权威处维护（如排障归日志页，参数归配置页）。其他地方需要提及时只用链接引用，禁止大段复制粘贴。
- **代码注释回归 What & How**：
  注释只阐述函数/类型的用途、输入输出与关键前提。严禁在注释里做哲学探讨、发表主观情绪评价（如“纯属噪音”、“没有意义”）或解释“为什么不按其他写法写”。

### 3. 较大改动要做完整单元测试

属于较大改动的情形：跨后端分层或同时改前后端、触及调度 / 投递 / 确认 / 鉴权等核心链路、
改模型或数据库、改公共接口契约、改动 ≥5 个源文件或 ≥200 行。

此时完整跑一遍：

```bash
cd backend && gofmt -l . && go vet ./... && go test ./... -race -count=1
cd frontend && npm run build && npm test
```

小改动至少跑相关包的 `go test`，前端则跑 `npx tsc --noEmit` 与相关测试。

测试要真跑通：新逻辑配新用例，不用 skip 或删用例掩盖失败，修 bug 先补复现用例。
汇报时列出实际执行的命令和结果。

### 4. 测试设计与文件收敛规范（严禁碎片化与无效测试）

- **测试文件严禁过度分散，能够合并的必须坚决合并**：
  - 同一个包内概念相近或同体系的测试用例，统一收敛在单个测试文件中，严禁按微小特性拆出碎片化文件。
  - **中间件测试全部合并在单个 `middleware_test.go`**：所有中间件（JWT、Token、CORS、BodyLimit、RateLimiter、RequestID、日志记录与错误恢复）统一在一个文件中覆盖。
  - **所有通知类型合并在单个 `channels_test.go`**：各类通知通道（Webhook、钉钉、企业微信、飞书、Bark、ntfy、Apprise、Telegram、SMTP 邮件等）统一在一个文件中覆盖，严禁每个通道类型单独开测试文件。
  - **其他基础模块保持单文件高聚合**：如 `database_test.go`、`router_test.go`、`logger_test.go`、`llm_test.go`、`models_test.go`、`scheduler_test.go`，一个包原则上只维护 1~2 个高内聚测试文件。
  - **严禁每次 PR 新建测试文件**：测试用例必须就近收敛在已有测试文件中；严禁为了单次 PR、阶段性联调、日志断言随手新建独立文件（如 `*_logging_test.go`、`*_flow_test.go`、`*.navigation.test.tsx` 等）。
- **后端测试下沉至逻辑层，严禁测 Handler**：
  - **禁止编写 Handler 层测试**：Handler 仅负责 Gin 上下文参数提取、状态码包装与 Service 串联，不承载独立业务规则；对 Handler 的冗余测试维护成本极高且易脱节。
  - **测试统一集中在逻辑层与核心机制**：测试聚焦在 `services/`、`scheduler/`、`notifier/`、`middleware/` 等包，通过内存数据库（`testutil.NewDB(t)`）验证真实业务逻辑、重试机制、时区与数据状态。
- **前端测试聚焦高价值状态与纯逻辑，拒绝无效 UI 冒烟**：
  - **禁止编写纯 UI 渲染与样式断言测试**：严禁断言 DOM 结构、class 属性、svg 个数或简单无状态展示文本（如 Header、EmptyState、Pagination、ConfirmDialog 等）。
  - **禁止编写 pages/ 页面级浅层集成测试**：严禁通过 mock 大量全局 API 来挂载整个页面仅为断言一两句静态文案或 Tab 切换，此类测试极易脆弱失效且产生 act 警告。
  - **前端测试仅限以下三类高价值场景**：
    1. `src/lib/` 纯函数工具库（时区时间格式化 `datetime`、接口错误解析 `apiError`、SSE 事件流解析 `aiStream`、通用工具 `utils`）。
    2. `src/contexts/` 核心全局状态管理（`AuthContext`、`ConfigContext`、`ThemeContext`）。
    3. 复杂状态机与数据转换组件（如 `AIChatWidget` 的流式中断/草稿暂存/会话缓存、`ChannelForms` 的渠道配置提取与反序列化）。

### 5. 严禁残留死代码与无用导出

代码库必须保持绝对精简，严禁留存任何无调用方的死代码、过度导出的符号或无用依赖：

- **函数、方法与类型随用随存**：
  删除或重构业务逻辑时，必须同步清理失去调用方的下游私有函数、辅助工具、常量与结构体定义，严禁留下“未来可能用到”的死代码。
- **导出权限最小化**：
  仅在当前包内部调用的 Go 函数、结构体与常量必须小写私有化；仅在当前模块/组件文件内引用的 TypeScript 函数、常量、变量或子组件严禁使用 `export`。
- **废弃接口与前端无用文件彻底清除**：
  废弃的前端 API 请求函数、已无调用的旧组件（`.tsx`）、空置类型或无用 npm 依赖必须直接彻底删除，严禁留存 0 引用文件或死依赖。
- **测试代码与业务代码同步收敛**：
  被测业务函数删除后，仅为其提供覆盖的单测用例与测试替身/脚手架函数必须同步删除，严禁留存仅自己测自己的孤儿测试。

## 其他约定

- **接口契约**：
  - 错误统一走 `middleware.AppError` + `abortErr`，响应格式 `{"error":{code,message,field?}}`；前端解析统一用 `extractApiError(err)` / `extractApiErrorCode(err)`。
  - 成功响应为 `{"code":200,"message":"ok","data":...}`。
  - 前端请求统一走 `src/lib/api.ts`，严禁在组件里直接使用 `axios`。
- **配置与安全**：
  - 敏感配置加密后入库，密钥与口令不得写进代码、日志、文档、提交信息。
  - 密码/Token/AI Key 读取时回显为 `***`；保存时回传 `***` 表示保留原密文，回传 `""` 表示清空，不得覆盖破坏已有密钥。
  - `/api/config` 仅限白名单键（`app_name`, `logo_svg`, `timezone`, `site_url`）；AI 设置严禁混入，必须走 `/api/ai/settings`。
- **日志规范**：
  - 用 `log/slog` 结构化输出。HTTP 请求链路内**必须使用 `logger.FromContext(ctx)`** 携带 `request_id`，不得裸调全局 `slog`。
  - 级别分级：成功访问日志降为 `debug`；请求耗时 >1 秒升为 `warn`；失败路径用 `warn`/`error`；`info` 仅用于生命周期与业务节点。
  - 记日志时 URL 必须通过 `redactURL` 脱敏 query，正文只记长度，严禁将凭据与敏感内容打入日志。
- **时间与时区**：
  - 后端：时间列一律用 `models.Time`（Unix 毫秒整数落库，写入自动归一化为 UTC），不用裸 `time.Time`；时间范围比较直接写 `col > ?`，不用 `julianday`。
  - 本地时间计算统一取 `timezone.Provider`（站点时区，设置页可改，存在 `config` 表 `key=timezone`），不按提醒保存。
  - 前端：时间展示统一走 `src/lib/datetime.ts`（取 `useSiteTimezone()`），严禁使用原生 `new Date().toLocaleString()`。
- **测试惯例**：
  - 内存 SQLite 与固定时区统一走 `internal/testutil`（`testutil.NewDB(t)`、`testutil.CST()`），各包不重复编写建库逻辑。
  - 新增测试用例就近追加到所属模块的已有测试文件中，严禁单开零散文件；严格遵守「测 Service 与核心机制，不测 Handler；测前端状态与算法工具，不测纯展示 UI」原则。
- **Git 提交**：
  - 提交信息用中文 + `feat/fix/refactor/docs/test/chore(scope):` 前缀。


