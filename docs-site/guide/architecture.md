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
时间列会额外做一次值转换（`normalizeTimeColumns`），把旧版本遗留的时间文本
就地转成整数毫秒，详见下文「时区处理」。

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

- 所有时间列在数据库里存 **Unix 毫秒整数**，不再是时间文本。
  模型层用 `models.Time`（内嵌 `time.Time`，实现 `driver.Valuer` / `sql.Scanner`）
  统一转换，写入时自动归一化为 UTC
- 时区是**站点级全局设置**（面板「设置」页，按 `config` 表 `key=timezone` 保存），不按提醒保存
- 调度计算时按站点时区解释 `schedule_spec` 里的本地时间（如 `09:00` 表示当地 9 点）
- 展示时前端按站点时区渲染（`lib/datetime.ts` 的 `formatInZone`），不依赖浏览器本地时区。
  接口返回的时间仍是 RFC3339 字符串，形态与早期版本一致
- 在设置页改时区会立刻重建 cron 实例、按新时区重算并回写所有启用提醒的 `next_fire_at`，无需重启
  （`Engine.ReloadTimezone` → `Engine.RecomputeNextFire`）。只重建不重算是不够的：
  sweeper 依据库里的 `next_fire_at` 判断补漏，旧值在新时区下会变成过去时间，
  导致错误补发或被误标 `expired`
- 重建 cron 实例时，每日日志清理等维护型任务会一并恢复（`Engine.purgeJobs`）

::: tip 为什么时间存整数而不是文本
早期版本直接用 `time.Time`，go-sqlite3 会写成
`2026-10-26 18:23:02.123456789+08:00` 这类变宽小数位、带时区偏移的文本。
SQLite 对这种列按字典序比较，两行偏移不同时同一时刻会被排到不同位置，
范围查询与排序都会失真。当时的补救是在 SQL 里套 `julianday()`，
但函数包裹列会让**索引完全失效**（退化为全表扫描），而且 `julianday` 是浮点天数，
等值比较只到约 50µs 精度，乐观锁的精确匹配形同虚设。

改存整数毫秒后：比较与排序都是整数语义，`(enabled, next_fire_at)` 复合索引
可直接走范围扫描，等值精确，也不必再靠"记得把时间转成 UTC"这条纪律维持正确性。

升级时 `AutoMigrate` 只改列类型、不转换既有值，因此启动阶段还会跑一次
`database.normalizeTimeColumns`，把残留的文本按值转成整数毫秒（解析不了的脏值置 NULL）。
:::

## 并发与一致性

| 场景 | 做法 |
|---|---|
| 主调度器与兜底扫描同时命中 | 事务内乐观锁，只有一个能推进 `next_fire_at` |
| 多通道并行发送 | 每个通道一个 goroutine，`WaitGroup` 汇总 |
| SQLite 写入 | 连接池限制为少量连接，WAL 模式下读可并发 |
| 批次删除 | 分块处理，避免 SQL 参数过多 |
