# Reminder — 个人提醒助手

开源地址：[github.com/hausen1012/reminder](https://github.com/hausen1012/reminder) ｜
文档地址：[hausen1012.github.io/reminder](https://hausen1012.github.io/reminder/)

一套自托管的全栈提醒系统：把要做的事写进去，到点自动通过你配置的渠道通知你，
你在手机上点一下确认即完成闭环。前后端一体打包为单个可执行文件，运行即服务。

## 特性

- **灵活调度**：单次、周期（分/时/天/月/年）、Cron 表达式，并支持公历与农历
- **多渠道通知**：SMTP 邮件、钉钉 / 企业微信 / 飞书机器人、Telegram、Bark、ntfy、Apprise、Webhook
- **确认闭环**：发送后通过 Token 链接确认，未确认自动重发
- **AI 助手**：用自然语言对话创建通道与提醒，兼容 Anthropic 与 OpenAI 协议（DeepSeek、Moonshot、Ollama 等），写操作需人工确认
- **断点补发**：进程重启后自动补发遗漏窗口内的触发，不漏提醒
- **全程留痕**：每次发送都有日志记录，异常一目了然
- **开箱即用**：内置登录认证与前端页面，单文件部署，无需外部依赖

## 快速开始

### 本地开发

需要 Go 与 Node.js。仓库提供了一键启动脚本，前端与后端会同时运行并支持前端热更新：

```bash
make dev
```

也可以分开启动：

```bash
make dev-backend    # 后端
make dev-frontend   # 前端（热更新）
```

### 质量检查

```bash
make fmt     # 格式化后端代码
make lint    # 静态检查
make test    # 前后端单元测试
```

CI 会在每次 push / PR 时执行这些检查。

### 构建与部署

#### Docker 部署（推荐）

使用官方多架构镜像（支持 amd64 / arm64）：

```bash
docker pull hausen1012/reminder:latest

docker run -d \
  --name reminder \
  --restart unless-stopped \
  -p 8765:8765 \
  -v ./reminder/db:/data/db \
  -v ./reminder/log:/var/log/reminder \
  -e SITE_URL=http://localhost:8765 \
  hausen1012/reminder:latest
```

或使用 Docker Compose 启动：

```bash
docker compose up -d
```

#### 本地构建

```bash
make build         # 构建单文件二进制到 build/
make docker        # 本地构建镜像并运行 Docker
```

### 配置

全部通过环境变量配置，可用项见 `.env.example`。生产环境至少应设置
管理员账号、`SITE_URL`（确认链接的域名，也可登录后到「设置 → 站点地址」修改）、`JWT_SECRET` 与 `ENCRYPTION_KEY`。

## 技术栈

- **后端**：Go + Gin + GORM + SQLite
- **前端**：React + TypeScript + Vite + Tailwind CSS
- **部署**：单文件二进制或 Docker 多阶段构建

## 文档

完整文档（指南、参考手册、外部 API）见文档站点：[hausen1012.github.io/reminder](https://hausen1012.github.io/reminder/)

本地预览文档：

```bash
cd docs-site && npm install
npm run docs:dev
```
