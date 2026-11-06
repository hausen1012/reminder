# 开发与测试

## 环境要求

| 工具 | 版本 |
|---|---|
| Go | 1.25 |
| Node.js | 20 以上 |

SQLite 驱动使用 CGO，本地构建需要 C 编译器（Windows 用 MinGW / TDM-GCC，macOS 用 Xcode CLT，
Linux 装 `gcc`）。

## 常用命令

| 命令 | 作用 |
|---|---|
| `make dev` | 一键启动前后端（前端热更新） |
| `make dev-backend` | 只启动后端 |
| `make dev-frontend` | 只启动前端 |
| `make build` | 构建当前平台的单文件二进制 |
| `make build-linux` / `build-macos` / `build-windows` | 交叉编译 |
| `make fmt` | 格式化 Go 代码 |
| `make lint` | `go vet` + 格式校验 |
| `make test` | 后端 Go 测试 + 前端 Vitest |
| `make docker` | 用 compose 构建并启动 |
| `make clean` | 清理构建产物 |

## 目录约定

```
backend/
  cmd/server/          入口，含 //go:embed web/*
  internal/
    config/            环境变量解析
    database/          连接与迁移
    logger/            全局 slog 配置
    models/            GORM 模型
    handlers/          HTTP 处理器
    services/          业务逻辑
    scheduler/         调度引擎与兜底扫描
    notifier/          各通道发送实现
    middleware/        鉴权、错误、CORS、限流
    timezone/          站点时区（运行期可改）
    llm/               厂商无关的对话接口（Anthropic / OpenAI 兼容实现）
    router/            路由装配
    crypto/secretbox/  对称加密
    testutil/          仅测试用的脚手架（内存库、固定时区）
frontend/
  src/
    pages/             路由页面
    components/
      ui/              Radix / shadcn 基础原语
      common/          项目自有通用组件（PageHeader、分页、确认框等）
      channels/ reminders/ tokens/ layout/
    contexts/  lib/  types/  test/
docs-site/             文档站点（VitePress）
```

前端展示时间统一走 `src/lib/datetime.ts`（`formatInZone` / `formatNextFireInZone`），
时区取自 `useSiteTimezone()`（站点全局设置）。不要直接用 `new Date(x).toLocaleString()`：
那会跟随浏览器本地时区，用户换设备后看到的时间会整体偏移。

## 后端测试

```bash
cd backend && go test ./...
```

CI 里会用 `-race` 跑一遍，因此涉及并发的改动建议本地也加上：

```bash
cd backend && go test ./... -race -count=1
```

测试覆盖的重点路径包括：调度乐观锁、兜底补发与过期、通道投递状态机、确认重发链、
外部 API 令牌隔离、确认链接转义、AI 工具编排与待确认操作。

AI 相关测试用 fake `llm.Provider` 注入脚本化响应（`services/ai_service_test.go`），
不访问真实网络；OpenAI 兼容协议本身用 `httptest` 起本地服务验证请求形状
（`llm/provider_test.go`）。浮窗的本地存储恢复、草稿角标等行为在
`components/ai/AIChatWidget.test.tsx` 里用 `sessionStorage` 覆盖。

写测试时的三个惯例：

- 内存 SQLite 建库与固定时区统一走 `internal/testutil`（`testutil.NewDB(t)`、
  `testutil.CST()`、`testutil.TZ(loc)`），各包不再各自抄一份建库代码
- 每个包共享的脚手架（fake 替身、造数辅助、等待函数）集中放在该包的
  `testutil_test.go`，不散落在具体用例文件里
- handler 的测试通过 `httptest` 打真实路由，而不是直接调用方法

## 前端测试

```bash
cd frontend && npm test           # 单次运行
cd frontend && npm run test:watch # 监听模式
cd frontend && npm run test:coverage
```

用 Vitest + Testing Library。组件测试优先按用户可见的文案和角色查询，
而不是断言样式类名。

## 新增一个通知通道

1. 在 `notifier/` 下实现 `Notifier` 接口（`Type()` 与 `Send()`）
2. 在 `notifier.go` 的 `Registry` 里注册
3. 在 `channel_service.go` 的 `validType` 与 `requireFields` 里加上类型与必填校验
4. 在前端 `ChannelEditDialog` 里挂上对应的表单组件
5. 补充发送逻辑与校验的单元测试

四处的类型字符串必须完全一致，否则运行时会报「未知通道类型」。

## 错误处理约定

所有错误都通过 `middleware.AppError` 表达，最终由 `ErrorHandler` 统一渲染为：

```json
{ "error": { "code": "validation_failed", "message": "标题必填", "field": "title" } }
```

handler 里只需 `abortErr(c, err)`，不要自己拼错误响应体。这样前端只需要实现一套错误解析逻辑。

`/api` 下未注册的路径由 `router.registerNotFound` 统一补成同样的结构，
因此新增路由时不需要额外处理 404。前端取值统一用 `lib/api.ts` 的
`extractApiError` / `extractApiErrorCode` / `extractApiErrorField`。

## 日志与凭据

日志会落到文件和容器 stdout，因此**不得写入任何凭据**。已在用的脱敏工具：

| 场景 | 做法 |
|---|---|
| 要记 URL | `notifier.redactURL` / `redactURLString`，会抹掉 query 取值与 userinfo |
| 要记 header | `notifier.headerKeys`，只出名字不出值 |
| 要记渲染后的正文 / 变量 | 只记长度（`body_len`、`vars_count`），不记内容 |

正文里可能带 `{{confirm_url}}`（一次性确认令牌），变量里还可能带用户内容，
记全文等于把令牌与隐私一起写进日志。`log` 通道是唯一例外——它的用途就是把正文打出来。

### 写日志的约定

- **级别要选对**：失败路径用 `Warn` / `Error`，不要用 `Info`——否则设置
  `LOG_LEVEL=warn` 时真正的故障会被一起藏掉。`Info` 只留给生命周期与业务节点。
- **错误必须带 `error` 字段**：`slog.Warn("...", "error", err)`，不要只写一句
  「失败了」。
- **HTTP 错误只记一次**：handler 用 `abortErr`，由 `ErrorHandler` 统一记录并分级；
  请求体解析失败用 `abortBindErr(c, err, "请求体格式错误")`，它会记下底层原因。
  不要在 handler 里再补一条日志。
- **带上下文**：请求内加 `request_id`（`middleware.RequestIDOf(c)`）；
  投递链路带 `reminder` / `log` / `channel` / `attempt`，便于 `grep` 串起来。
- **调试细节用 `Debug`**：高频成功路径（每条 SQL、单通道发送成功）不要用 `Info`，
  否则日志会被淹没。

## 文档站点

```bash
cd docs-site
npm install
npm run docs:dev      # 本地预览，热更新
npm run docs:build    # 构建静态站点到 .vitepress/dist
```

站点在 push 到 `main` 时由 GitHub Actions 自动构建并发布到 GitHub Pages。
