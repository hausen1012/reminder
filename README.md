# Reminder — 个人提醒助手

全栈一体化个人提醒/通知系统，支持多通道发送、公历/农历调度。

## 特性

- **多种调度方式**：支持单次、周期（分/时/天/月/年）、Cron 表达式、农历（单次+周期）
- **多通道通知**：SMTP 邮件、钉钉机器人、企业微信机器人、Webhook
- **确认机制**：发送后需点击 Token 链接确认，未确认自动重发
- **兜底容错**：进程重启后自动补发错过窗口内的触发，不漏提醒
- **完整日志**：每次发送都有记录，异常一目了然
- **开箱即用**：内置登录认证，前端嵌入 Go 单文件，运行即服务

## 快速开始

### 本地开发

```bash
# 启动后端
make dev-backend

# 新终端，启动前端（热更新）
make dev-frontend
```

然后打开 http://localhost:5173。

### 质量检查

```bash
# 格式化后端代码
make fmt

# 静态检查（go vet + gofmt 格式校验）
make lint

# 单元测试（后端 Go + 前端 Vitest）
make test
```

CI 在每次 push / PR 时自动执行格式检查、静态检查与全部单元测试，见 `.github/workflows/ci.yml`。

### 生产构建

```bash
# 构建单文件二进制
make build
./build/server

# 或 Docker 部署
make docker
```

### 配置

通过环境变量配置，参考 `.env.example`：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| PORT | 8765 | 监听端口 |
| DB_PATH | /data/db/reminder.db | 数据库路径 |
| JWT_SECRET | 随机生成 | JWT 签名密钥，留空时每次启动随机生成（重启后旧令牌失效） |
| ENCRYPTION_KEY | 内置 fallback | 通道敏感字段加密密钥（base64 编码的 32 字节），生产环境务必配置 |
| BASE_URL | http://localhost:8765 | 公网地址（确认链接用） |
| TIMEZONE | Asia/Shanghai | 系统时区 |
| SWEEP_INTERVAL_SEC | 60 | 兜底扫描间隔（秒） |
| MISS_TOLERANCE_MINUTES | 60 | 漏触发容忍窗口（超过标记过期） |
| LOG_AUTO_PURGE_DAYS | 0 | 日志自动清理天数（0=不自动清理） |
| LOG_FILE | /var/log/reminder | 日志目录；未配置时写入该目录，同时输出到 stderr |
| USERNAME | admin | 初始管理员用户名 |
| PASSWORD | admin123 | 初始管理员密码 |

## 测试

| 范围 | 命令 | 覆盖率 |
|------|------|--------|
| 后端 | `cd backend && go test ./...` | middleware 100% / services 83.3% / notifier 83.2% / secretbox 83.3% / scheduler 80.3% / handlers 70.3% |
| 前端 | `cd frontend && npm test` | 168 个用例；contexts 97% / components 71~100% / lib 79.6% |

前端覆盖率用 `npm run test:coverage` 查看，按模块统计：

| 模块 | 语句覆盖 |
|------|----------|
| `components/guards` | 100% |
| `components/layout` | 100% |
| `contexts` | 97% |
| `lib`（api / utils） | 79.6% |
| `components/channels` | 71.7% |
| `pages` / `App.tsx` | 0%（未测，见下方说明） |

CI 会用 `-race` 跑后端测试，覆盖调度乐观锁、sweeper 补漏与过期、通道投递状态机、确认重发链、
外部 API 令牌隔离、确认链接 XSS 转义等核心路径。

未覆盖的部分：整页组件（`pages/*`，单文件 200~800 行、依赖大量接口调用与路由状态）、
真实外部通道（SMTP/钉钉/企微）的网络调用。这两类改动成本高、收益低，
如需覆盖建议先用 `@testing-library/react` 配 MSW 做接口 mock 后再拆页。

## 技术栈

- **前端**：React + TypeScript + Vite + Tailwind CSS + shadcn/ui
- **后端**：Go + Gin + GORM + SQLite
- **部署**：Docker 多阶段构建