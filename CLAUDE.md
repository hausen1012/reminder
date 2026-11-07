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

## 三条硬性要求

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
- **Git 提交**：
  - 提交信息用中文 + `feat/fix/refactor/docs/test/chore(scope):` 前缀。


