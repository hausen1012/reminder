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
    models/            GORM 模型
    handlers/          HTTP 处理器
    services/          业务逻辑
    scheduler/         调度引擎与兜底扫描
    notifier/          各通道发送实现
    middleware/        鉴权、错误、CORS、限流
    crypto/secretbox/  对称加密
frontend/
  src/
    pages/  components/  contexts/  lib/  ui/
docs-site/             文档站点（VitePress）
docs/                  设计与实施记录
```

## 后端测试

```bash
cd backend && go test ./...
```

CI 里会用 `-race` 跑一遍，因此涉及并发的改动建议本地也加上：

```bash
cd backend && go test ./... -race -count=1
```

测试覆盖的重点路径包括：调度乐观锁、兜底补发与过期、通道投递状态机、确认重发链、
外部 API 令牌隔离、确认链接转义。

写测试时的两个惯例：

- 用内存 SQLite 建库，通过 `testutil_test.go` 里的辅助函数构造数据
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

## 文档站点

```bash
cd docs-site
npm install
npm run docs:dev      # 本地预览，热更新
npm run docs:build    # 构建静态站点到 .vitepress/dist
```

站点在 push 到 `main` 时由 GitHub Actions 自动构建并发布到 GitHub Pages。
