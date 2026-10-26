# 响应格式与错误码

所有接口的响应结构是统一的，客户端只需要实现一套解析逻辑。

## 成功响应

```json
{
  "code": 200,
  "message": "ok",
  "data": { }
}
```

`code` 固定为 `200`，业务数据都在 `data` 里。部分接口的 `data` 是列表容器：

```json
{
  "code": 200,
  "message": "ok",
  "data": { "items": [], "total": 0 }
}
```

也有接口在成功时不返回数据，此时 `data` 为 `null`。

## 错误响应

```json
{
  "error": {
    "code": "validation_failed",
    "message": "标题必填",
    "field": "title"
  }
}
```

`field` 是可选的，指出具体出错的字段，便于前端高亮定位。

## 错误码对照

| HTTP | `error.code` | 含义 |
|---|---|---|
| 400 | `validation_failed` | 参数校验失败，通常是请求体不合法 |
| 401 | `unauthorized` | 未登录、登录已过期，或令牌无效 / 缺失 |
| 403 | `forbidden` | 已鉴权但无权访问 |
| 404 | `not_found` | 资源不存在，或不属于当前调用方 |
| 409 | `conflict` | 与现有数据冲突（如名称重复） |
| 429 | `rate_limited` | 请求过于频繁，外部 API 默认每令牌 60 次/分钟 |
| 500 | `internal_error` | 服务器内部错误 |

::: tip 所有错误出口都统一
业务错误、鉴权中间件拒绝、路由不存在、panic 恢复，返回的都是上面这个结构。
因此客户端只需读 `response.data.error.message` 就能拿到可展示的文案，
读 `response.data.error.field` 就能定位出错字段。
:::

## 分页约定

列表接口统一用 `limit` + `offset` 查询参数：

| 参数 | 默认 | 上限 |
|---|---|---|
| `limit` | 因接口而异，通常 50 | 200 |
| `offset` | 0 | — |

返回体带 `total` 表示符合条件的总条数，用于前端算总页数。

## 排序约定

支持排序的列表接口接受：

| 参数 | 取值 |
|---|---|
| `sort_by` | 因接口而异（如 `created_at`、`next_fire_at`、`title`） |
| `sort_order` | `asc` / `desc` |

传入不支持的字段时会忽略并回退到默认排序，不会报错。

## 时间格式

| 场景 | 格式 |
|---|---|
| JSON 时间字段（`created_at`、`next_fire_at` 等） | RFC3339，UTC，如 `2026-07-01T01:00:00Z` |
| 给前端直接展示的便捷字段（如 `next_fire_at_local`） | `YYYY-MM-DD HH:mm`，已转成提醒自身时区 |
| 调度规则里的时间（`schedule_spec.at`） | `YYYY-MM-DDTHH:mm:ss`，按提醒时区解释 |

数据库统一存 UTC，时区只影响计算与展示。
