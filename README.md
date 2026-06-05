# Reminder2 — 个人提醒助手

全栈一体化个人提醒/通知系统，支持多通道发送、公历/农历调度、外部 API 调用。

## 特性

- **提醒调度**：单次、周期（分/时/天/月/年）、Cron、农历（单次+周期）
- **多通道通知**：SMTP 邮件、钉钉机器人、企业微信机器人、Webhook（GET/POST）
- **确认机制**：可选"需要确认"，一次性 Token 链接，未确认自动重发
- **外部 API**：API Key 鉴权，支持外部程序创建提醒
- **容错兜底**：进程重启后 sweeper 自动补发错过窗口内的触发
- **完整日志**：发送记录、通道尝试详情、按时间清理
- **全栈一体化**：前端嵌入 Go 二进制，单文件运行
- **开箱即用**：内置登录认证、用户管理、主题切换

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
# 本地二进制
make build
./build/server

# 或 Docker 部署
make docker
```

### 配置

通过环境变量配置，参考 `.env.example`：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| PORT | 8080 | 监听端口 |
| DB_PATH | /data/db/bedrock.db | 数据库路径 |
| JWT_SECRET | 自动生成 | JWT 签名密钥 |
| SECRET_BOX_KEY | 硬编码常量 | 通道敏感字段加密密钥（base64） |
| PUBLIC_BASE_URL | http://localhost:8080 | 公网地址（确认链接用） |
| TIMEZONE | Asia/Shanghai | 系统时区 |
| SWEEP_INTERVAL_SEC | 60 | 兜底扫描间隔（秒） |
| MISS_TOLERANCE_MINUTES | 60 | 漏触发容忍窗口（超过标记 expired） |
| LOG_AUTO_PURGE_DAYS | 0 | 日志自动清理天数（0=不自动清理） |
| INIT_USERNAME | admin | 初始管理员 |
| INIT_PASSWORD | admin123 | 初始密码 |

## API 端点

### 面板 API（JWT 认证）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/health | 健康检查 |
| POST | /api/auth/login | 登录 |
| GET/POST | /api/reminders | 提醒列表/创建 |
| POST | /api/reminders/preview | 预览下次触发时间 |
| GET | /api/reminders/upcoming | 未来待发提醒 |
| GET/PUT/DELETE | /api/reminders/:id | 单条提醒 |
| PATCH | /api/reminders/:id/toggle | 启用/禁用 |
| POST | /api/reminders/:id/test | 立即试发 |
| GET/POST | /api/channels | 通道列表/创建 |
| GET/PUT/DELETE | /api/channels/:id | 单条通道 |
| PATCH | /api/channels/:id/toggle | 启用/禁用 |
| POST | /api/channels/:id/test | 试发测试 |
| GET | /api/channels/stats | 通道发送统计 |
| GET | /api/logs | 日志列表 |
| GET | /api/logs/:id | 日志详情 |
| DELETE | /api/logs | 清理日志 |
| GET/POST | /api/apikeys | API Key 列表/创建 |
| GET | /api/apikeys/stats | API Key 调用统计 |

### Ingest API（API Key 认证）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/ingest/reminders | 创建提醒 |
| GET | /api/ingest/reminders | 列出本 Key 的提醒 |
| GET | /api/ingest/reminders/:id | 查询提醒 |
| DELETE | /api/ingest/reminders/:id | 删除提醒 |
| GET | /api/ingest/docs | API 文档 |

### 公开

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /c/:token | 确认链接（无需认证） |

## 技术栈

- **前端**: React 18 + TypeScript + Vite + Tailwind CSS + shadcn/ui
- **后端**: Go 1.21+ + Gin + GORM + SQLite
- **部署**: Docker 多阶段构建