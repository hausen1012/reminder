# 架构说明

## 整体形态

```
                    ┌─────────────────────────────┐
   浏览器 ───────────│  Go 进程（单二进制）        │
   手机 ────────────▶│                             │
                    │  Gin 路由                   │
                    │   ├── 静态文件（嵌入前端）  │
                    │   ├── /api/*  面板接口      │
                    │   ├── /api/external/v1/*    │
                    │   └── /c/:token 确认链接    │
                    │                             │
                    │  服务层                     │
                    │   ├── ReminderService       │
                    │   ├── ChannelService        │
                    │   ├── DispatchService       │
                    │   ├── ConfirmService        │
                    │   └── ...                   │
                    │                             │
                    │  调度层                     │
                    │   ├── Engine（cron+定时器） │
                    │   └── Sweeper（兜底扫描）   │
                    │                             │
                    │  SQLite（单文件）           │
                    └────────────┬────────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │ SMTP / 钉钉 / 企微 / HTTP │
                    └─────────────────────────┘
```

前端是 React SPA，构建产物通过 `go:embed` 打进二进制，由同一个进程提供静态文件服务。
因此**部署时只有一个可执行文件和一个数据库文件**。

## 后端分层

| 层 | 职责 |
|---|---|
| `handlers` | HTTP 入参解析、调用服务、输出统一响应体 |
| `services` | 业务逻辑：CRUD、校验、渲染、投递、确认链 |
| `scheduler` | 下次触发计算、触发器注册、兜底扫描 |
| `notifier` | 各通道的发送实现，统一 `Notifier` 接口 |
| `models` | GORM 模型 |
| `middleware` | 鉴权、日志、错误渲染、CORS、限流 |
| `crypto/secretbox` | 通道敏感字段的对称加解密 |

设计上有两条约束值得注意：

- **服务层不依赖 scheduler 的具体实现**，而是依赖一个 `SchedulerHook` 接口，避免循环依赖
- **调度器不直接发送消息**，而是写一条 `pending` 日志后异步交给 `DispatchService`，
  把「什么时候触发」和「怎么发出去」解耦

## 一次触发的完整链路

```
Engine 定时器到期
  │
  ├─ 事务内用乐观锁推进 next_fire_at（防止重复触发）
  ├─ 插入 delivery_log（status=pending）
  └─ 异步调用 DispatchService.Run
        │
        ├─ 加载提醒绑定的启用通道（无通道 → failed）
        ├─ 构建模板变量
        ├─ 需要确认 → 创建/复用确认链与令牌，注入 confirm_url
        ├─ 渲染标题与正文
        ├─ 各通道并发发送，单通道内部按节奏重试
        ├─ 每个通道的每次尝试写 delivery_attempt
        └─ 汇总所有通道结果 → delivery_log.status
```

`delivery_log.status` 的取值由通道结果汇总而来：全部成功是 `success`，
全部失败是 `failed`，部分成功是 `partial`。

## 数据模型

| 表 | 说明 |
|---|---|
| `users` | 管理员账号 |
| `reminders` | 提醒主体，含调度规则与下次触发时间 |
| `channels` | 通知通道配置（敏感字段加密） |
| `reminder_channels` | 提醒与通道的多对多关联 |
| `delivery_logs` | 每次触发的总记录 |
| `delivery_attempts` | 每个通道每次尝试的明细 |
| `confirm_tokens` | 确认令牌 |
| `tokens` | 外部 API 令牌 |
| `token_default_channels` | 令牌的默认通道路由 |
| `configs` | 站点配置（键值对） |

数据库迁移在启动时自动执行（`AutoMigrate`），新增字段自动补齐。

## 前端结构

| 目录 | 说明 |
|---|---|
| `pages` | 页面级组件：首页、提醒、通知、日志、令牌、监控、设置 |
| `components` | 可复用组件，按功能分目录 |
| `contexts` | 认证、主题、站点配置的全局状态 |
| `lib` | API 封装与工具函数 |
| `ui` | 基础 UI 组件 |

路由使用 `HashRouter`，这样静态文件服务不需要为每个前端路由配置回退规则。

## 时区处理

- 数据库里存的时间统一为 UTC
- 调度计算时按提醒自身的 `timezone`（默认取系统 `TIMEZONE`）解释本地时间
- 展示时再转成本地时间，同时提供 `next_fire_at_local` 之类的便捷字段

## 并发与一致性

| 场景 | 做法 |
|---|---|
| 主调度器与兜底扫描同时命中 | 事务内乐观锁，只有一个能推进 `next_fire_at` |
| 多通道并行发送 | 每个通道一个 goroutine，`WaitGroup` 汇总 |
| SQLite 写入 | 连接池限制为少量连接，WAL 模式下读可并发 |
| 批次删除 | 分块处理，避免 SQL 参数过多 |
