# AGENTS.md

## 一、通用开发规范

### 1. 文档、注释与文案精简（拒绝废话与心路历程）

- **客观陈述事实，不写心路历程**：文档只讲功能、用法、边界与规则。严禁记录设计讨论、自我辩解（如“刻意不做”、“为什么不提供”）或阶段性历史（如“Phase X 接入”、“优化了旧版缺陷”等 PR 话术）。
- **界面文案力求简炼**：提示信息、帮助说明、弹窗文案一句话讲清核心用途与指引，严禁将底层内部机制与排障碎碎念直接暴露给用户。
- **严控 FAQ 门槛，拒绝说理与分散维护**：FAQ 仅收录高频硬性排障步骤；常规参数限制与架构定位直接写入正文或表格，禁止开 FAQ 讲道理，禁止跨页面重复维护。
- **注释回归 What & How**：仅说明用途、入参出参及关键前提，严禁主观情绪评价或辩解写法。

### 2. UI 与交互设计哲学（布局即解释）

- **布局即解释，严禁标签堆砌**：依靠视觉层级、分组边界、留白间距与色彩语义自然传达优先级与从属；凡能直观理解的内容，严禁滥用 Tag、Badge、副标题、Helper Text 或 Tooltip。
- **交互自解释，拒绝说明书式设计**：把精力投入在即时反馈、就地校验、明晰的主次对比与错误恢复引导上，让用户凭直觉直接操控，拒绝把设计缺陷甩给使用说明。
- **视觉对齐优于绝对几何对齐**：以人眼视觉平衡（Optical Alignment）为准，针对图标重心、字体行高与基线差异主动微调补偿，不机械依赖坐标绝对居中。

### 3. 代码架构设计与复用规范（拒绝造轮子与面条代码）

- **不熟悉功能严禁从零臆造**：遇到不熟悉的业务功能或技术领域，禁止直接从零凭空实现。必须优先检索 GitHub 同类优质开源项目，对比成熟实现方案，汲取经生产验证的实践后再动手。Vibe Coding 必须包含主动调研查证过程。
- **先查后写，严禁重复造轮子**：编写新功能、工具函数或通用组件前，先检索现有代码库，优先复用或适度扩展，杜绝重复实现或随手复制。
- **职责单一与主动抽取**：严禁编写状态纠缠的面条代码；多处重复或夹杂复杂转换/过滤/流程的逻辑，坚决提炼为独立函数、通用类或公共模块。

### 4. 严禁残留死代码与无用导出

- **死代码随用随清**：重构或删除逻辑时，同步清理失去调用的下游私有函数、常量、结构体/类定义与无用依赖，不留“未来可能用到”的代码。
- **导出权限最小化**：仅包内/模块内调用的符号必须保持私有，严禁无外部依赖的无节制导出（最小化 public / export 范围）。
- **废弃资源彻底清除**：无引用的废弃模块、旧接口与无用第三方依赖直接物理删除，保持 0 冗余。
- **测试代码同步收敛**：业务代码删除后，对应的测试用例与测试脚手架同步删除，不留孤儿测试。

### 5. 测试设计通用原则

- **测试下沉核心逻辑，不测浅层胶水层**：控制器、路由胶水层仅负责参数透传与状态包装，不承载业务规则；测试统一聚焦在业务逻辑层与领域服务，验证真实规则与状态流转。
- **聚焦状态与核心逻辑，拒绝脆弱浅层冒烟**：严禁断言纯静态结构、样式或编写全量 Mock 的浅层冒烟测试；测试资源集中在纯算法工具、全局状态转换与复杂状态机。
- **测试真跑通与完整覆盖**：新逻辑配新用例，修 Bug 补复现用例，严禁使用 skip 或删除断言掩盖失败。跨架构分层、改核心链路、改对外契约/数据库、改动较大（≥5 文件或 ≥200 行）时必须全量跑通。

### 6. Git 提交规范

- 遵循 Conventional Commits 格式规范：`feat/fix/refactor/docs/test/chore(scope): 描述信息`。

---

## 二、本项目专有规范

### 1. 系统与语言

- 本项目全程使用中文对话、编写文档（含 Commit、计划、总结）与代码注释（PowerShell 脚本除外）。

### 2. 项目概况与架构

自托管提醒系统：Go + Gin + GORM + SQLite 后端，React + TypeScript + Vite 前端。
前端构建产物经 `go:embed` 打进二进制，部署为单个可执行文件 + 一个数据库文件。

```
backend/    cmd/server 入口，internal/ 下分 handlers / services / scheduler / notifier / middleware / testutil 等
frontend/   src/pages、components、contexts、lib/api.ts
docs-site/  对外文档站（VitePress），是对外唯一权威文档
```

- **常用命令**：`make dev` / `make fmt` / `make lint` / `make test` / `make build` / `make docker`。
- **构建产物与临时文件（严禁提交）**：`backend/cmd/server/web/`、`frontend/dist/`、`build/`、`*.db`、`logs/`。

### 3. 代码整洁细则（语言特定）

- **Go 权限最小化**：仅包内部调用的函数、结构体与常量必须首字母小写私有化。
- **前端权限最小化**：仅组件/模块内使用的 TypeScript 变量、函数与子组件严禁加 `export`。
- **清理干净**：无引用的旧组件（`.tsx`）与废弃 API 请求函数彻底删除。

### 4. 接口与通信契约

- **错误响应**：统一走 `middleware.AppError` + `abortErr`，响应格式 `{"error":{code,message,field?}}`；前端解析统一使用 `extractApiError(err)` / `extractApiErrorCode(err)`。
- **成功响应**：统一格式为 `{"code":200,"message":"ok","data":...}`。
- **前端请求**：统一走 `src/lib/api.ts`，严禁在组件里直接使用 `axios`。

### 5. 配置与安全

- **敏感配置**：加密后入库，密钥与口令严禁写进代码、日志、文档或提交信息。
- **密钥回显与保存**：密码 / Token / AI Key 读取时回显为 `***`；保存时回传 `***` 表示保留原密文，回传 `""` 表示清空，不得覆盖破坏已有密钥。
- **配置接口白名单**：`/api/config` 仅限白名单键（`app_name`, `logo_svg`, `timezone`, `site_url`）；AI 设置严禁混入，必须走 `/api/ai/settings`。

### 6. 日志规范 (slog)

- **结构化输出**：统一采用 `log/slog`。HTTP 请求链路内**必须使用 `logger.FromContext(ctx)`** 携带 `request_id`，不得裸调全局 `slog`。
- **日志级别**：成功访问降为 `debug`；耗时 >1 秒升为 `warn`；失败路径用 `warn` / `error`；`info` 仅用于生命周期与核心业务节点。
- **敏感信息脱敏**：URL 必须通过 `redactURL` 脱敏 query，正文只记长度，严禁将凭据与敏感内容打入日志。

### 7. 时间与时区

- **后端存储**：时间列一律用 `models.Time`（Unix 毫秒整数落库，写入自动归一化为 UTC），不用裸 `time.Time`；时间范围比较直接写 `col > ?`，不用 `julianday`。
- **本地时间计算**：统一通过 `timezone.Provider` 获取站点时区（存储于 `config` 表 `key=timezone`），不按提醒单独保存时区。
- **前端展示**：时间展示统一走 `src/lib/datetime.ts`（通过 `useSiteTimezone()` 获取），严禁直接使用原生 `new Date().toLocaleString()`。

### 8. 项目测试落地与收敛

- **测试基础设施**：内存 SQLite 与固定时区统一走 `internal/testutil`（`testutil.NewDB(t)`、`testutil.CST()`），各包不重复编写建库逻辑。
- **后端测试下沉与文件收敛**：
  - **严禁编写 Handler 层测试**：Handler 仅负责 Gin 上下文参数提取、状态码包装与 Service 串联，测试统一聚焦在 `services/`、`scheduler/`、`notifier/`、`middleware/`。
  - **中间件测试全部合并在单个 `middleware_test.go`**：覆盖 JWT、Token、CORS、BodyLimit、RateLimiter、RequestID、日志与恢复。
  - **所有通知类型合并在单个 `channels_test.go`**：覆盖 Webhook、钉钉、企微、飞书、Bark、ntfy、Apprise、Telegram、SMTP 等。
  - **基础模块保持单文件高内聚**：`database_test.go`、`router_test.go`、`logger_test.go` 等，严禁为单次 PR / 阶段联调新建零散文件。
- **前端测试落点与排除**：
  - **聚焦高价值**：`src/lib/`（`datetime`, `apiError`, `aiStream`, `utils`）、`src/contexts/`（`AuthContext` 等）、复杂状态组件（`AIChatWidget`、`ChannelForms`）。
  - **严禁无效测试**：禁止断言 DOM/class/无状态展示，禁止 pages/ 页面级浅层集成测试。
- **必须完整跑通的场景**：触及调度/投递/确认/鉴权核心链路、改前后端/模型/DB、改动 ≥5 文件或 ≥200 行。
- **执行命令**：
  ```bash
  cd backend && gofmt -l . && go vet ./... && go test ./... -race -count=1
  cd frontend && npm run build && npm test
  ```

### 9. 文档同步对照表

代码改动时主动判断并说明文档同步情况（「更新了哪些文档」或「无需更新及原因」）：
- 接口 / 字段 / 错误码变化 → `docs-site/`
- 环境变量变化 → `.env.example`、`docs-site/guide/configuration.md`
- 构建脚本变化 → `README.md`、`docs-site/guide/development.md`
- 仅内部实现重构（不影响对外行为）无需更新文档，但要在回复中说明。
