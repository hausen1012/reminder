// ingest 处理外部 API 调用（/api/ingest/*）。
//
// 与面板使用的 ReminderHandler 共享 ReminderService，但鉴权使用 API Key 而非 JWT。
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
	ApiKeySvc   *services.ApiKeyService
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

	// 未指定通道时回退到 API Key 默认通道
	apiKeyID, _ := c.Get("api_key_id")
	if keyID, ok := apiKeyID.(uint); ok {
		in.APIKeyID = &keyID
		if len(in.ChannelIDs) == 0 {
			in.ChannelIDs = h.ApiKeySvc.DefaultChannelIDs(keyID)
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
	if !h.belongsToKey(v.APIKeyID, c) {
		abortErr(c, middleware.NewAppError(middleware.CodeNotFound, "提醒不存在"))
		return
	}
	successJSON(c, v)
}

// ListReminders GET /api/ingest/reminders
func (h *IngestHandler) ListReminders(c *gin.Context) {
	apiKeyID, _ := c.Get("api_key_id")
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
	if keyID, ok := apiKeyID.(uint); ok {
		f.APIKeyID = &keyID
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
	if !h.belongsToKey(v.APIKeyID, c) {
		abortErr(c, middleware.NewAppError(middleware.CodeNotFound, "提醒不存在"))
		return
	}
	if err := h.ReminderSvc.Delete(id); err != nil {
		abortErr(c, err)
		return
	}
	successJSON(c, nil)
}

// Docs GET /api/ingest/docs
func (h *IngestHandler) Docs(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(ingestDocsHTML))
}

func (h *IngestHandler) belongsToKey(reminderKeyID *uint, c *gin.Context) bool {
	apiKeyID, exists := c.Get("api_key_id")
	if !exists {
		return false
	}
	keyID, ok := apiKeyID.(uint)
	if !ok {
		return false
	}
	if reminderKeyID == nil {
		return false
	}
	return *reminderKeyID == keyID
}

const ingestDocsHTML = `<!DOCTYPE html>
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
code{background:#e5e7eb;padding:2px 6px;border-radius:4px;font-size:13px}
pre{background:#1e293b;color:#e2e8f0;padding:16px;border-radius:8px;overflow-x:auto;font-size:13px;line-height:1.5;margin:8px 0}
table{width:100%;border-collapse:collapse;margin:12px 0;font-size:14px}
th,td{text-align:left;padding:8px 12px;border-bottom:1px solid #e5e7eb}
th{background:#f9fafb;font-weight:600}
.tag{display:inline-block;padding:2px 8px;border-radius:4px;font-size:12px;font-weight:600;margin-right:6px}
.tag-post{background:#22c55e;color:#fff}
.tag-get{background:#3b82f6;color:#fff}
.tag-delete{background:#ef4444;color:#fff}
</style>
</head>
<body>
<h1>Ingest API 文档</h1>
<p>通过 API Key 鉴权的外部调用接口，用于程序化创建和管理提醒。</p>

<h2>鉴权</h2>
<p>所有请求需在 HTTP 头携带 API Key：<code>X-API-Key: bdrk_xxxxxxxx</code></p>
<p>API Key 由面板创建，创建时仅展示一次。</p>

<h2>端点</h2>

<h3><span class="tag tag-post">POST</span> /api/ingest/reminders</h3>
<p>创建一条提醒。</p>
<pre>curl -X POST /api/ingest/reminders \
  -H "X-API-Key: bdrk_xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "每日提醒",
    "content": "现在是 {{now}}",
    "calendar": "solar",
    "schedule_type": "cron",
    "schedule_spec": {"expr": "0 9 * * *"},
    "timezone": "Asia/Shanghai",
    "channel_ids": [1],
    "require_confirm": false
  }'</pre>
<p>未指定 <code>channel_ids</code> 时将使用 API Key 绑定的默认通道。</p>

<h3><span class="tag tag-get">GET</span> /api/ingest/reminders</h3>
<p>列出本 Key 创建的提醒（仅返回 enabled 的）。</p>
<pre>curl /api/ingest/reminders?limit=20&offset=0 \
  -H "X-API-Key: bdrk_xxx"</pre>

<h3><span class="tag tag-get">GET</span> /api/ingest/reminders/:id</h3>
<p>查看单条提醒详情。</p>
<pre>curl /api/ingest/reminders/1 \
  -H "X-API-Key: bdrk_xxx"</pre>

<h3><span class="tag tag-delete">DELETE</span> /api/ingest/reminders/:id</h3>
<p>删除本 Key 创建的提醒。</p>
<pre>curl -X DELETE /api/ingest/reminders/1 \
  -H "X-API-Key: bdrk_xxx"</pre>

<h2>字段说明</h2>
<table>
<tr><th>字段</th><th>类型</th><th>必填</th><th>说明</th></tr>
<tr><td>title</td><td>string</td><td>是</td><td>提醒标题，最长 200 字符</td></tr>
<tr><td>content</td><td>string</td><td>否</td><td>提醒内容，支持 <code>{{var}}</code> 模板</td></tr>
<tr><td>calendar</td><td>string</td><td>是</td><td><code>solar</code> 或 <code>lunar</code></td></tr>
<tr><td>schedule_type</td><td>string</td><td>是</td><td><code>once</code> / <code>interval</code> / <code>cron</code></td></tr>
<tr><td>schedule_spec</td><td>object</td><td>是</td><td>调度参数，参见下文</td></tr>
<tr><td>timezone</td><td>string</td><td>否</td><td>时区，默认 Asia/Shanghai</td></tr>
<tr><td>channel_ids</td><td>int[]</td><td>否</td><td>通知通道 ID，为空时用 Key 默认通道</td></tr>
<tr><td>require_confirm</td><td>bool</td><td>否</td><td>是否需要确认</td></tr>
<tr><td>confirm_retry_interval_sec</td><td>int</td><td>否</td><td>重发间隔（秒）</td></tr>
<tr><td>confirm_max_retries</td><td>int</td><td>否</td><td>最大重发次数</td></tr>
</table>

<h2>schedule_spec 格式</h2>
<table>
<tr><th>Calendar</th><th>Type</th><th>schedule_spec 示例</th></tr>
<tr><td>solar</td><td>once</td><td><code>{"at": "2026-06-06T09:00:00"}</code></td></tr>
<tr><td>solar</td><td>interval</td><td><code>{"every": 1, "unit": "day"}</code></td></tr>
<tr><td>solar</td><td>cron</td><td><code>{"expr": "0 9 * * *"}</code></td></tr>
<tr><td>lunar</td><td>once</td><td><code>{"year": 2026, "month": 1, "day": 1, "hour": 9, "minute": 0}</code></td></tr>
<tr><td>lunar</td><td>interval</td><td><code>{"start_year": 2026, "start_month": 1, "start_day": 1, "every": 1, "unit": "month"}</code></td></tr>
</table>
</body>
</html>`