# 内置 API

面板使用的 HTTP 接口。鉴权方式是 `Authorization: Bearer <jwt>`，登录接口除外。

多数情况下你不需要直接调这些接口——外部程序应该用[外部 API](/api/external)。
这里列出它们是为了便于排查问题与二次开发。

## 认证

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/auth/login` | 登录，返回 JWT 与用户信息 |
| `GET` | `/api/auth/me` | 当前登录用户 |
| `POST` | `/api/auth/logout` | 登出（JWT 无状态，由前端丢弃令牌） |
| `PUT` | `/api/auth/password` | 修改密码，需提供原密码 |

登录请求体：

```json
{ "username": "admin", "password": "admin123" }
```

JWT 有效期为 72 小时。改密码后旧 JWT 仍然有效直到过期——如需立即失效，重启服务并更换 `JWT_SECRET`。

## 提醒

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/reminders` | 列表，支持 `source` / `enabled` / `search` / 排序 / 分页 |
| `POST` | `/api/reminders` | 新建 |
| `GET` | `/api/reminders/:id` | 详情 |
| `PUT` | `/api/reminders/:id` | 更新（全量覆盖） |
| `DELETE` | `/api/reminders/:id` | 硬删除 |
| `DELETE` | `/api/reminders/batch` | 批量删除，请求体 `{ "ids": [1,2] }` |
| `PATCH` | `/api/reminders/:id/toggle` | 启用 / 禁用 |
| `POST` | `/api/reminders/preview` | 预览接下来的触发时间 |
| `POST` | `/api/reminders/test-dry` | 用表单数据试发，不写日志 |
| `GET` | `/api/reminders/upcoming` | 即将触发的提醒 |

`upcoming` 支持两种查询方式：

- `?within=24h&limit=10`：未来一段时间内的提醒
- `?scope=today`：只今天，不分页

`preview` 请求体：

```json
{
  "calendar": "solar",
  "schedule_type": "cron",
  "schedule_spec": { "expr": "0 9 * * *" },
  "timezone": "Asia/Shanghai",
  "count": 5
}
```

返回 `{ "times": [...] }`，最多 20 条。

## 通知通道

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/channels` | 列表。不带参数返回全量数组；带 `limit` 等参数返回 `{items,total}` |
| `POST` | `/api/channels` | 新建 |
| `GET` | `/api/channels/:id` | 详情（敏感字段脱敏为 `***`） |
| `PUT` | `/api/channels/:id` | 更新（与旧配置合并，未传的字段保持原值） |
| `DELETE` | `/api/channels/:id` | 删除 |
| `DELETE` | `/api/channels/batch` | 批量删除 |
| `PATCH` | `/api/channels/:id/toggle` | 启用 / 禁用 |
| `POST` | `/api/channels/test-dry` | 用表单配置测试发送 |
| `GET` | `/api/channels/stats` | 各通道使用统计 |

更新通道时 `config` 走的是**合并**语义：未传的键保持原值，传了的键才覆盖。

`_enc` 结尾的敏感字段有两种特殊值：

| 传值 | 效果 |
|---|---|
| `***` | 沿用原密文，不修改 |
| `""` | 清空该字段 |

前端编辑表单在密码框留空时回传的就是 `***`，因此「不动密码框」不会误删密码。

## 日志

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/logs` | 列表，支持 `status` / `source` / `search` / `reminder_id` / `since` / 分页 |
| `GET` | `/api/logs/:id` | 详情，含通道级 attempts 与确认链接 |
| `GET` | `/api/logs/count` | 按清理条件统计待删条数 |
| `DELETE` | `/api/logs` | 清理，`?older_than=7d` 或 `?all=true` |
| `DELETE` | `/api/logs/batch` | 批量删除指定日志 |

`older_than` 接受 Go 时长格式，如 `168h`（7 天）、`720h`（30 天），前端会把天数换算成小时。

## 令牌

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/tokens` | 列表，支持 `search` 与分页 |
| `POST` | `/api/tokens` | 新建，返回明文 |
| `GET` | `/api/tokens/:id` | 详情 |
| `GET` | `/api/tokens/:id/plaintext` | 取回明文 |
| `PATCH` | `/api/tokens/:id/toggle` | 启用 / 禁用 |
| `PUT` | `/api/tokens/:id/channels` | 修改默认通道 |
| `DELETE` | `/api/tokens/:id` | 删除 |
| `GET` | `/api/tokens/stats` | 各令牌近 24 小时调用次数 |

## 监控与配置

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/health` | 健康检查，**无需鉴权** |
| `GET` | `/api/scheduler/status` | 调度器状态与注册明细，支持分页 |
| `GET` | `/api/config` | 站点配置键值对 |
| `PUT` | `/api/config` | 批量更新配置，值为空串表示删除该键 |

## 确认链接

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/c/:token` | 消费确认令牌，**无需鉴权**，返回一个提示页面 |

这个路径不走 API 前缀，返回的是 HTML 页面而非 JSON，方便在手机浏览器里直接点开。

## 静态文件

除 `/api/*` 与 `/c/*` 之外的路径都由嵌入的前端静态文件服务处理。
前端使用 HashRouter，因此不需要服务端为前端路由配置回退规则。
