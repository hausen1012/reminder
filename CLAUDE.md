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

## 两条硬性要求

### 1. 代码改动要确认文档是否更新

每次改完代码，主动判断是否需要同步文档，并在回复里明确说明「更新了哪些文档」或「无需更新及原因」。

主要对照：

- 接口 / 字段 / 错误码变化 → `docs-site/`
- 环境变量变化 → `.env.example`、`docs-site/guide/configuration.md`
- 构建脚本变化 → `README.md`、`docs-site/guide/development.md`

只改内部实现、不影响对外行为的重构，无需更新文档，但要说明。

### 2. 较大改动要做完整单元测试

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

- 错误统一走 `middleware.AppError` + `abortErr`，响应格式 `{"error":{code,message,field?}}`；成功响应为 `{"code":200,"message":"ok","data":...}`。
- 日志用 `log/slog` 结构化输出。
- 时间以 UTC 入库，本地时间计算用 `cfg.Location`。
- 前端请求统一走 `src/lib/api.ts`，不在组件里直接 `axios`。
- 敏感配置加密后入库，密钥与口令不得写进代码、日志、文档、提交信息。
- 提交信息用中文 + `feat/fix/refactor/docs/test/chore(scope):` 前缀。

