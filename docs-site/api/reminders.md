# 提醒接口

## 创建

`POST /api/external/v1/reminders`

### 公历单次

```bash
curl -X POST http://localhost:8765/api/external/v1/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "会议提醒",
    "content": "15 分钟后有会议",
    "schedule_type": "once",
    "schedule_spec": { "at": "2026-07-01T14:00:00" },
    "channel_ids": [1]
  }'
```

### 公历 Cron

```bash
curl -X POST http://localhost:8765/api/external/v1/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "每日站会",
    "schedule_type": "cron",
    "schedule_spec": { "expr": "0 9 * * 1-5" },
    "channel_ids": [1, 2]
  }'
```

### 公历周期

```bash
curl -X POST http://localhost:8765/api/external/v1/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "每小时提醒",
    "schedule_type": "interval",
    "schedule_spec": { "start_at": "2026-07-01T09:00:00", "every": 1, "unit": "hour" }
  }'
```

这里没有传 `channel_ids`，会发到令牌的默认通道。

### 农历单次

```bash
curl -X POST http://localhost:8765/api/external/v1/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "春节提醒",
    "calendar": "lunar",
    "schedule_type": "once",
    "schedule_spec": {
      "lunar": { "year": 2027, "month": 1, "day": 1 },
      "hour": 9,
      "minute": 0
    },
    "channel_ids": [1]
  }'
```

### 农历周期

```bash
curl -X POST http://localhost:8765/api/external/v1/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "每月初一提醒",
    "calendar": "lunar",
    "schedule_type": "interval",
    "schedule_spec": {
      "start_lunar": { "year": 2026, "month": 1, "day": 1 },
      "hour": 9,
      "minute": 0,
      "every": 1,
      "unit": "month"
    },
    "channel_ids": [1]
  }'
```

### 需要确认的提醒

```bash
curl -X POST http://localhost:8765/api/external/v1/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "吃药",
    "content": "记得吃药，吃完点下面链接确认",
    "schedule_type": "cron",
    "schedule_spec": { "expr": "0 8 * * *" },
    "require_confirm": true,
    "confirm_retry_interval_sec": 600,
    "confirm_max_retries": 3
  }'
```

## schedule_spec 速查

| 日历 | 类型 | schedule_spec |
|---|---|---|
| `solar` | `once` | `{"at": "2026-07-01T09:00:00"}` |
| `solar` | `interval` | `{"start_at": "2026-07-01T09:00:00", "every": 1, "unit": "day"}` |
| `solar` | `cron` | `{"expr": "0 9 * * *"}` |
| `lunar` | `once` | `{"lunar": {"year": 2027, "month": 1, "day": 1}, "hour": 9, "minute": 0}` |
| `lunar` | `interval` | `{"start_lunar": {...}, "hour": 9, "minute": 0, "every": 1, "unit": "month"}` |

`unit` 取值：公历支持 `minute` / `hour` / `day` / `month` / `year`；
农历支持 `day` / `month` / `year`。

## 列出提醒

`GET /api/external/v1/reminders`

| 参数 | 说明 |
|---|---|
| `limit` | 每页条数，默认 50 |
| `offset` | 偏移量，默认 0 |
| `search` | 搜索标题与正文关键字 |

```bash
curl "http://localhost:8765/api/external/v1/reminders?limit=10&offset=0" \
  -H "X-AUTH: bdrk_xxx"
```

```json
{
  "code": 200,
  "message": "ok",
  "data": { "items": [ ], "total": 1 }
}
```

只返回本令牌创建的提醒。

## 查看单条

`GET /api/external/v1/reminders/:id`

```bash
curl http://localhost:8765/api/external/v1/reminders/1 \
  -H "X-AUTH: bdrk_xxx"
```

响应结构同创建接口的 `data`。

## 更新

`PUT /api/external/v1/reminders/:id`

请求体字段与创建接口完全相同。更新按**全量**处理：`title`、`calendar`、`schedule_type`、
`schedule_spec` 都是必填，缺任何一个都会返回 `validation_failed`。

```bash
curl -X PUT http://localhost:8765/api/external/v1/reminders/1 \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "修改后的标题",
    "calendar": "solar",
    "schedule_type": "cron",
    "schedule_spec": { "expr": "0 10 * * *" },
    "content": "改了调度时间",
    "channel_ids": [1]
  }'
```

::: warning 更新是全量覆盖
只想改标题，也要把 `calendar`、`schedule_type`、`schedule_spec` 一起带上。
另外 `channel_ids` 会被整体替换：传 `[]` 或不传都会清空该提醒的通道绑定。

未传 `enabled` 时保留当前启用状态，其余字段则按传入值覆盖。
:::

## 删除

`DELETE /api/external/v1/reminders/:id`

```bash
curl -X DELETE http://localhost:8765/api/external/v1/reminders/1 \
  -H "X-AUTH: bdrk_xxx"
```

```json
{ "code": 200, "message": "ok", "data": null }
```

删除是硬删除，同时清理关联与调度器注册项。
