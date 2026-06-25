// ingest 处理外部 API 调用（/api/ingest/*）。
//
// 与面板使用的 ReminderHandler 共享 ReminderService，但鉴权使用令牌而非 JWT。
// 提醒创建时强制 source=api，未指定通道时回退到 Key 的默认通道。
package handlers

import (
	"net/http"
	"strconv"

	"github.com/bedrock/backend/internal/middleware"
	"github.com/bedrock/backend/internal/services"
	"github.com/gin-gonic/gin"
)

// IngestHandler 是外部 Ingest API。
type IngestHandler struct {
	ReminderSvc *services.ReminderService
	TokenSvc    *services.TokenService
	ChannelSvc  *services.ChannelService
}

// CreateReminder POST /api/ingest/reminders
func (h *IngestHandler) CreateReminder(c *gin.Context) {
	var in services.ReminderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		abortErr(c, middleware.NewAppError(middleware.CodeValidationFailed, "请求体格式错误"))
		return
	}

	// 强制 source=api
	in.Source = "api"

	// 未指定通道时回退到令牌默认通道
		tokenID, _ := c.Get("token_id")
		if keyID, ok := tokenID.(uint); ok {
			in.TokenID = &keyID
			if len(in.ChannelIDs) == 0 {
				in.ChannelIDs = h.TokenSvc.DefaultChannelIDs(keyID)
			}
		}

	v, err := h.ReminderSvc.Create(in)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, v)
}

// GetReminder GET /api/ingest/reminders/:id
func (h *IngestHandler) GetReminder(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	v, err := h.ReminderSvc.Get(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	// 限制只能查看本 Key 创建的
	if !h.belongsToKey(v.TokenID, c) {
		abortErr(c, middleware.NewAppError(middleware.CodeNotFound, "提醒不存在"))
		return
	}
	successJSON(c, v)
}

// ListReminders GET /api/ingest/reminders
func (h *IngestHandler) ListReminders(c *gin.Context) {
	tokenID, _ := c.Get("token_id")
	f := services.ListFilter{
		Search: c.Query("search"),
	}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}
	if keyID, ok := tokenID.(uint); ok {
		f.TokenID = &keyID
	}

	items, total, err := h.ReminderSvc.List(f)
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, gin.H{"items": items, "total": total})
}

// DeleteReminder DELETE /api/ingest/reminders/:id
func (h *IngestHandler) DeleteReminder(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		abortErr(c, err)
		return
	}
	// 先查，确认是本 Key 创建的
	v, err := h.ReminderSvc.Get(id)
	if err != nil {
		abortErr(c, err)
		return
	}
	if !h.belongsToKey(v.TokenID, c) {
		abortErr(c, middleware.NewAppError(middleware.CodeNotFound, "提醒不存在"))
		return
	}
	if err := h.ReminderSvc.Delete(id); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}

// ListChannels GET /api/ingest/channels
//
// 返回所有通知渠道列表，供外部调用方选择 channel_id。
func (h *IngestHandler) ListChannels(c *gin.Context) {
	views, err := h.ChannelSvc.List()
	if err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, views)
}

// Docs GET /api/ingest/docs
func (h *IngestHandler) Docs(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderIngestDocs()))
}

func (h *IngestHandler) belongsToKey(reminderKeyID *uint, c *gin.Context) bool {
	tokenID, exists := c.Get("token_id")
	if !exists {
		return false
	}
	keyID, ok := tokenID.(uint)
	if !ok {
		return false
	}
	if reminderKeyID == nil {
		return false
	}
	return *reminderKeyID == keyID
}

func renderIngestDocs() string {
	return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Ingest API 文档</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#f5f5f5;color:#333;padding:40px;max-width:960px;margin:0 auto}
h1{font-size:28px;margin-bottom:8px}
p{color:#666;margin-bottom:24px;line-height:1.6}
h2{font-size:20px;margin:32px 0 12px;padding-bottom:6px;border-bottom:2px solid #e5e7eb}
h3{font-size:16px;margin:20px 0 8px;color:#444}
h4{font-size:14px;margin:16px 0 6px;color:#555}
code{background:#e5e7eb;padding:2px 6px;border-radius:4px;font-size:13px}
pre{background:#1e293b;color:#e2e8f0;padding:16px;border-radius:8px;overflow-x:auto;font-size:13px;line-height:1.5;margin:8px 0}
pre.req{background:#0f3b2e;border-left:3px solid #22c55e}
pre.res{background:#1e1a33;border-left:3px solid #a78bfa}
pre.curl{background:#1e293b}
table{width:100%;border-collapse:collapse;margin:12px 0;font-size:14px}
th,td{text-align:left;padding:8px 12px;border-bottom:1px solid #e5e7eb}
th{background:#f9fafb;font-weight:600}
.tag{display:inline-block;padding:2px 8px;border-radius:4px;font-size:12px;font-weight:600;margin-right:6px}
.tag-post{background:#22c55e;color:#fff}
.tag-get{background:#3b82f6;color:#fff}
.tag-delete{background:#ef4444;color:#fff}
.field-opt{color:#999;font-size:12px}
</style>
</head>
<body>
<h1>Ingest API 文档</h1>
<p>通过令牌鉴权的外部调用接口，用于程序化创建和管理提醒。</p>

<h2>鉴权</h2>
<p>所有请求需在 HTTP 头携带令牌：<code>X-AUTH: bdrk_xxxxxxxx</code></p>
<p>令牌由面板创建，创建时仅展示一次。</p>

<h2>通用响应格式</h2>
<p>所有接口返回统一 JSON 结构：</p>
<pre class="res">{
  "code": 0,         // 0=成功，非0=错误码
  "message": "ok",   // 成功或错误描述
  "data": { ... }    // 业务数据，详见各接口
}</pre>

<table>
<tr><th>错误码</th><th>说明</th></tr>
<tr><td>0</td><td>成功</td></tr>
<tr><td>401</td><td>令牌无效或未提供</td></tr>
<tr><td>429</td><td>请求频率超限</td></tr>
<tr><td>40001</td><td>参数校验失败</td></tr>
<tr><td>40401</td><td>资源不存在</td></tr>
</table>

<hr style="margin:32px 0">

<h2>端点</h2>

<!-- ──────── POST /api/ingest/reminders ──────── -->
<h3><span class="tag tag-post">POST</span> /api/ingest/reminders</h3>
<p>创建一条提醒。</p>

<h4>请求体</h4>
<pre class="req">{
  "title":        "string",       // 必填，最长 200 字符
  "content":      "string",       // 选填，默认等于 title
  "schedule_spec": {
    "at":    "2026-06-06T09:00:00", // once 类型
    "expr":  "0 9 * * *",           // cron 类型
    "every": 1,                     // interval 类型
    "unit":  "day"                  // interval 类型：day|hour|minute
  },
  "schedule_type":           "once",  // once | interval | cron，默认 once
  "calendar":                "solar", // solar | lunar，默认 solar
  "timezone":       "Asia/Shanghai",  // 选填，默认 Asia/Shanghai
  "channel_ids":            [1, 2],   // 选填，为空时使用令牌默认通道
  "require_confirm":        false,    // 选填
  "confirm_retry_interval_sec": 60,   // 选填
  "confirm_max_retries":         3    // 选填
}</pre>

<h4>响应</h4>
<pre class="res">{
  "id": 1,
  "title": "每日提醒",
  "content": "现在是 12:00",
  "content_format": "text",
  "calendar": "solar",
  "schedule_type": "cron",
  "schedule_spec": {"expr": "0 9 * * *"},
  "timezone": "Asia/Shanghai",
  "enabled": true,
  "source": "api",
  "token_id": 1,
  "require_confirm": false,
  "confirm_retry_interval_sec": 60,
  "confirm_max_retries": 3,
  "next_fire_at": "2026-06-25T01:00:00Z",
  "last_fired_at": null,
  "fire_count": 0,
  "channel_ids": [1],
  "created_at": "2026-06-24T12:00:00Z",
  "updated_at": "2026-06-24T12:00:00Z"
}</pre>

<!-- ──────── GET /api/ingest/reminders ──────── -->
<h3><span class="tag tag-get">GET</span> /api/ingest/reminders</h3>
<p>列出本令牌创建的提醒。</p>

<h4>查询参数</h4>
<table>
<tr><th>参数</th><th>类型</th><th>说明</th></tr>
<tr><td>limit</td><td>int</td><td>每页条数，默认 10</td></tr>
<tr><td>offset</td><td>int</td><td>偏移量，默认 0</td></tr>
<tr><td>search</td><td>string</td><td>搜索标题关键字</td></tr>
</table>

<h4>响应</h4>
<pre class="res">{
  "items": [
    {
      "id": 1,
      "title": "每日提醒",
      "content": "内容",
      "content_format": "text",
      "calendar": "solar",
      "schedule_type": "cron",
      "schedule_spec": {"expr": "0 9 * * *"},
      "timezone": "Asia/Shanghai",
      "enabled": true,
      "source": "api",
      "token_id": 1,
      "require_confirm": false,
      "confirm_retry_interval_sec": 60,
      "confirm_max_retries": 3,
      "next_fire_at": "2026-06-25T01:00:00Z",
      "last_fired_at": null,
      "fire_count": 0,
      "channel_ids": [1],
      "created_at": "2026-06-24T12:00:00Z",
      "updated_at": "2026-06-24T12:00:00Z"
    }
  ],
  "total": 1
}</pre>

<!-- ──────── GET /api/ingest/reminders/:id ──────── -->
<h3><span class="tag tag-get">GET</span> /api/ingest/reminders/:id</h3>
<p>查看单条提醒详情。只能查看本令牌创建的提醒。</p>

<h4>响应</h4>
<p>同 <code>POST&nbsp;/api/ingest/reminders</code> 的响应结构。</p>

<!-- ──────── DELETE /api/ingest/reminders/:id ──────── -->
<h3><span class="tag tag-delete">DELETE</span> /api/ingest/reminders/:id</h3>
<p>删除本令牌创建的提醒。</p>

<h4>响应</h4>
<pre class="res">{
  "code": 0,
  "message": "ok",
  "data": null
}</pre>

<!-- ──────── GET /api/ingest/channels ──────── -->
<h3><span class="tag tag-get">GET</span> /api/ingest/channels</h3>
<p>获取所有通知渠道列表。</p>

<h4>响应</h4>
<pre class="res">[
  {
    "id": 1,
    "name": "邮件通知",
    "type": "smtp",
    "enabled": true,
    "config": {
      "host": "***",
      "port": "***"
    },
    "created_at": "2026-06-24T12:00:00Z",
    "updated_at": "2026-06-24T12:00:00Z"
  }
]</pre>

<p>注意：<code>config</code> 中敏感字段以 <code>_enc</code> 结尾的值会被脱敏为 <code>"***"</code>。</p>

<h2>schedule_spec 格式</h2>
<table>
<tr><th>Calendar</th><th>schedule_type</th><th>schedule_spec 示例</th></tr>
<tr><td>solar</td><td>once</td><td><code>{"at": "2026-06-06T09:00:00"}</code></td></tr>
<tr><td>solar</td><td>interval</td><td><code>{"every": 1, "unit": "day"}</code></td></tr>
<tr><td>solar</td><td>cron</td><td><code>{"expr": "0 9 * * *"}</code></td></tr>
<tr><td>lunar</td><td>once</td><td><code>{"year": 2026, "month": 1, "day": 1, "hour": 9, "minute": 0}</code></td></tr>
<tr><td>lunar</td><td>interval</td><td><code>{"start_year": 2026, "start_month": 1, "start_day": 1, "every": 1, "unit": "month"}</code></td></tr>
</table>

<h2>curl 示例</h2>

<h4>创建一次性提醒</h4>
<pre class="curl">curl -X POST /api/ingest/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "会议提醒",
    "content": "15分钟后有会议",
    "schedule_spec": {"at": "2026-06-25T14:00:00"},
    "channel_ids": [1]
  }'</pre>

<h4>创建 CRON 提醒</h4>
<pre class="curl">curl -X POST /api/ingest/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "每日站会",
    "schedule_type": "cron",
    "schedule_spec": {"expr": "0 9 * * 1-5"},
    "timezone": "Asia/Shanghai",
    "channel_ids": [1, 2]
  }'</pre>

<h4>创建间隔重复提醒</h4>
<pre class="curl">curl -X POST /api/ingest/reminders \
  -H "X-AUTH: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "每小时提醒",
    "schedule_type": "interval",
    "schedule_spec": {"every": 1, "unit": "hour"},
    "channel_ids": [1]
  }'</pre>

<h4>列出提醒</h4>
<pre class="curl">curl "/api/ingest/reminders?limit=10&offset=0" \
  -H "X-AUTH: bdrk_xxx"</pre>

<h4>获取通知渠道</h4>
<pre class="curl">curl /api/ingest/channels \
  -H "X-AUTH: bdrk_xxx"</pre>

</body>
</html>`
}