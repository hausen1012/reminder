# 外部 API

通过令牌鉴权的外部调用接口，用于程序化创建与管理提醒。

## 鉴权

所有请求需要在 HTTP 头携带令牌：

```
X-AUTH: bdrk_xxxxxxxx
```

令牌在面板的**令牌**页创建。与面板登录用的 JWT 是两套独立机制，
详见[令牌与外部调用](/guide/tokens)。

## 限流

每个令牌每分钟最多 60 次请求，超出返回 `429 rate_limited`。计数在进程内存里，重启会重置。

## 通用约定

- 请求与响应都是 JSON，`Content-Type: application/json`
- 时间字段使用 `YYYY-MM-DDTHH:mm:ss` 的本地时间（按提醒的 `timezone` 解释）
- 只能访问**本令牌创建的**提醒，访问其他提醒一律返回「提醒不存在」
- 分页用 `limit` / `offset`

响应格式与错误码见[响应格式与错误码](/reference/response-and-errors)。

## 端点一览

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/external/v1/reminders` | 创建提醒 |
| `GET` | `/api/external/v1/reminders` | 列出本令牌创建的提醒 |
| `GET` | `/api/external/v1/reminders/:id` | 查看单条 |
| `PUT` | `/api/external/v1/reminders/:id` | 更新 |
| `DELETE` | `/api/external/v1/reminders/:id` | 删除 |
| `GET` | `/api/external/v1/channels` | 获取通道列表（用于挑选 `channel_ids`） |

外部 API 不能修改通道，只能读取。

## 创建提醒

`POST /api/external/v1/reminders`

### 请求体

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `title` | string | 是 | 标题，最长 200 字符 |
| `content` | string | 否 | 正文，留空时回退为标题 |
| `content_format` | string | 否 | `text` / `markdown` / `html`，默认 `text` |
| `calendar` | string | 否 | `solar` / `lunar`，默认 `solar` |
| `schedule_type` | string | 否 | `once` / `interval` / `cron`，默认 `once` |
| `schedule_spec` | object | 是 | 调度规则，结构随类型变化 |
| `timezone` | string | 否 | 默认取系统时区 |
| `channel_ids` | number[] | 否 | 留空时使用令牌的默认通道 |
| `enabled` | boolean | 否 | 默认 `true` |
| `require_confirm` | boolean | 否 | 是否需要确认，默认 `false` |
| `confirm_retry_interval_sec` | number | 否 | 重发间隔，最小 60 |
| `confirm_max_retries` | number | 否 | 最大重发次数，至少 1 |

`require_confirm` 为 `true` 时，后两个字段必须满足下限。

### 响应

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "id": 1,
    "title": "每日提醒",
    "content": "现在是 12:00",
    "content_format": "text",
    "calendar": "solar",
    "schedule_type": "cron",
    "schedule_spec": { "expr": "0 9 * * *" },
    "timezone": "Asia/Shanghai",
    "enabled": true,
    "source": "api",
    "token_id": 1,
    "require_confirm": false,
    "confirm_retry_interval_sec": 0,
    "confirm_max_retries": 0,
    "next_fire_at": "2026-07-01T01:00:00Z",
    "last_fired_at": null,
    "fire_count": 0,
    "next_fire_at_local": "2026-07-01 09:00",
    "channel_ids": [1],
    "created_at": "2026-06-30T12:00:00Z",
    "updated_at": "2026-06-30T12:00:00Z"
  }
}
```

`source` 恒为 `api`，`token_id` 是创建它的令牌 ID。`next_fire_at` 是 UTC 时间，
`next_fire_at_local` 是同一时刻在提醒时区下的展示形式。
