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
| JWT_SECRET | changeme | JWT 签名密钥 |
| ENCRYPTION_KEY | changeme | 通道敏感字段加密密钥（base64） |
| BASE_URL | http://localhost:8765 | 公网地址（确认链接用） |
| TIMEZONE | Asia/Shanghai | 系统时区 |
| SWEEP_INTERVAL_SEC | 60 | 兜底扫描间隔（秒） |
| MISS_TOLERANCE_MINUTES | 60 | 漏触发容忍窗口（超过标记过期） |
| LOG_AUTO_PURGE_DAYS | 0 | 日志自动清理天数（0=不自动清理） |
| USERNAME | admin | 初始管理员用户名 |
| PASSWORD | admin123 | 初始管理员密码 |

## 技术栈

- **前端**：React + TypeScript + Vite + Tailwind CSS + shadcn/ui
- **后端**：Go + Gin + GORM + SQLite
- **部署**：Docker 多阶段构建