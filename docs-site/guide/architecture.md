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
| `crypto/secretbox` | 通道敏感字段与 AI 草稿的对称加解密 |
| `llm` | 厂商无关的对话接口，Anthropic 与 OpenAI 兼容协议两个实现 |

两条约束：服务层依赖 `SchedulerHook` 接口而非 scheduler 具体实现，避免循环依赖；
调度器只写 `pending` 日志，发送异步交给 `DispatchService`，把「何时触发」和「怎么发」解耦。

## AI 助手的执行链路

```
POST /api/ai/chat（无状态：history 由前端上传）
  │
  ├─ 读取 ai_* 设置，按 provider 构造 llm.Provider
  ├─ 拼接客户端上传的历史（最多 30 条）
  └─ 工具循环（最多 8 轮）
        ├─ 只读工具立即执行，结果回填
        └─ 写工具只生成 ai_actions 草稿（payload_enc 加密），不落业务表
  │
  └─ 返回 reply + 本轮新生成的草稿

POST /api/ai/actions/:id/apply
  └─ 解密 payload → 调用既有 ReminderService / ChannelService
```

草稿执行走的是和面板表单相同的服务层方法，校验规则、调度器同步、通道绑定处理完全一致。
厂商差异收敛在 `internal/llm`：上层只看到 `Turn` / `ToolCall` / `Reply`；
Anthropic 开启思考时需原样回放上一轮的原始响应，通过 `Turn.Raw` 透传。

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
| `configs` | 站点配置（键值对），AI 设置也以 `ai_` 前缀存在这里 |
| `ai_actions` | AI 生成的待确认写操作（入参加密存储），保留 90 天后自动清理 |

数据库结构在启动时自动建齐（`AutoMigrate`）。

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

- 所有时间列在数据库里存 **Unix 毫秒整数**，模型层用 `models.Time` 统一转换，写入时归一化为 UTC
- 时区是**站点级全局设置**（`config` 表 `key=timezone`），不按提醒保存
- 调度计算按站点时区解释 `schedule_spec` 里的本地时间（如 `09:00` 表示当地 9 点）
- 展示时前端按站点时区渲染（`lib/datetime.ts` 的 `formatInZone`），不依赖浏览器本地时区。
  接口返回的时间统一是 RFC3339 字符串
- 改时区会立刻重建 cron 实例、按新时区重算并回写所有启用提醒的 `next_fire_at`，无需重启。
  sweeper 依据库里的 `next_fire_at` 判断补漏，所以只重建不重算会让旧值变成过去时间
- 重建 cron 实例时，每日日志清理等维护型任务会一并恢复（`Engine.purgeJobs`）

::: tip 为什么时间存整数而不是文本
SQLite 对文本列按字典序比较，带偏移的时间文本会让范围查询与排序失真；在 SQL 里套 `julianday()`
又会让索引失效。存整数毫秒后比较与排序都是整数语义，`(enabled, next_fire_at)` 复合索引可直接走范围扫描。
:::

## 并发与一致性

| 场景 | 做法 |
|---|---|
| 主调度器与兜底扫描同时命中 | 事务内乐观锁，只有一个能推进 `next_fire_at` |
| 多通道并行发送 | 每个通道一个 goroutine，`WaitGroup` 汇总 |
| 进程在投递中途退出 | 兜底扫描把超过 10 分钟仍是 `pending` 的日志收敛为 `failed`，并写明原因 |
| 进程重启后的确认重发 | 启动时按库里每条链的轮次与上次发送时间重排下一轮，已流逝的时间从间隔里扣掉 |
| SQLite 写入 | 连接池限制为少量连接，WAL 模式下读可并发 |
| 批次删除 | 分块处理，避免 SQL 参数过多 |
