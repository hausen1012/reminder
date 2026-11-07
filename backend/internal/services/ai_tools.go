package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/reminder/backend/internal/llm"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
)

// ai_tools 定义暴露给模型的工具，以及只读工具的执行逻辑。
//
// 写操作（创建通道 / 创建、修改、删除提醒）一律不直接落库：
// 工具只生成一条 AIAction 草稿，由用户在前端点确认后才真正执行。
// 这样模型误解意图时不会静默改动数据，也不会绕过服务层的校验。

// toolDefinition 是工具的 schema 与执行函数。
type toolDefinition struct {
	spec llm.Tool
	run  func(ctx context.Context, input json.RawMessage) (string, error)
}

// 开启「需要确认」但模型没给重发参数时的兜底值，与提醒表单的默认值一致。
//
// 用户常说「这个提醒需要确认」而不提间隔与次数；工具层补默认值比让服务层
// 报「重发间隔最小 60 秒」更符合预期，模型也不用猜。
const (
	defaultConfirmRetryIntervalSec = 300
	defaultConfirmMaxRetries       = 3
)

// channelTypeSchema 供 create_channel 使用。
var channelTypeSchema = map[string]any{
	"type":        "string",
	"enum":        []string{"smtp", "dingtalk", "wecom", "webhook", "log"},
	"description": "通道类型。smtp=邮件，dingtalk=钉钉机器人，wecom=企业微信机器人，webhook=通用 Webhook，log=仅打印到服务器日志",
}

// scheduleSpecSchemaDescription 说明三种调度类型各自的 schedule_spec 结构。
const scheduleSpecSchemaDescription = `调度参数，按 schedule_type 取用：
- once：{"at":"2006-01-02T15:04:05"}（站点时区本地时间）
- interval：{"start_at":"2006-01-02T15:04:05","every":1,"unit":"minute|hour|day|month|year"}
- cron：{"expr":"0 9 * * *"}（5 字段标准 cron，按站点时区解释）
农历规则用 calendar=lunar，把 at/start_at 换成 lunar/start_lunar:{"year":2026,"month":1,"day":1}，另配 hour/minute。`

// toolDefinitions 返回暴露给模型的工具列表。
func (s *AIService) toolDefinitions() []toolDefinition {
	return []toolDefinition{
		{
			spec: llm.Tool{
				Name:        "list_channels",
				Description: "列出当前已配置的通知通道（id、名称、类型、是否启用）。创建提醒前先调用它确认可用的 channel_ids。",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			},
			run: s.toolListChannels,
		},
		{
			spec: llm.Tool{
				Name:        "list_reminders",
				Description: "列出已有提醒（最多 20 条），可按关键词过滤。修改或删除提醒前先调用它拿到 id。",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"search": map[string]any{"type": "string", "description": "按标题或内容模糊匹配"},
					},
				},
			},
			run: s.toolListReminders,
		},
		{
			spec: llm.Tool{
				Name: "create_reminder",
				Description: "提出创建一条提醒。不会立即创建，会生成一张待用户确认的卡片。" +
					"必须提供 title、schedule_type、schedule_spec、channel_ids。" +
					"用户要求「需要确认 / 没确认就催我」时传 require_confirm=true。",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":          map[string]any{"type": "string", "description": "提醒标题，最长 200 字"},
						"content":        map[string]any{"type": "string", "description": "提醒正文，留空则与标题相同"},
						"content_format": map[string]any{"type": "string", "enum": []string{"text", "markdown", "html"}},
						"calendar":       map[string]any{"type": "string", "enum": []string{"solar", "lunar"}, "description": "默认 solar"},
						"schedule_type":  map[string]any{"type": "string", "enum": []string{"once", "interval", "cron"}},
						"schedule_spec":  map[string]any{"type": "object", "description": scheduleSpecSchemaDescription},
						"channel_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "通知通道 id 列表，来自 list_channels"},
						"require_confirm": map[string]any{
							"type": "boolean",
							"description": "是否需要确认。用户要求「需要确认 / 没点确认就重发 / 直到我确认」时必须传 true，" +
								"并配置下面两个重发参数（不传则用默认 300 秒、3 次）",
						},
						"confirm_retry_interval_sec": map[string]any{
							"type": "integer", "minimum": 60,
							"description": "确认重发间隔（秒），最小 60，默认 300。仅 require_confirm=true 时有意义",
						},
						"confirm_max_retries": map[string]any{
							"type": "integer", "minimum": 1,
							"description": "确认最大重发次数，至少 1，默认 3。仅 require_confirm=true 时有意义",
						},
					},
					"required": []string{"title", "schedule_type", "schedule_spec", "channel_ids"},
				},
			},
			run: func(ctx context.Context, raw json.RawMessage) (string, error) {
				return s.toolCreateReminder(ctx, raw)
			},
		},
		{
			spec: llm.Tool{
				Name:        "update_reminder",
				Description: "提出修改一条已有提醒。id 来自 list_reminders，其余字段含义同 create_reminder；未提供的字段保持原值。",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":            map[string]any{"type": "integer"},
						"title":         map[string]any{"type": "string"},
						"content":       map[string]any{"type": "string"},
						"schedule_type": map[string]any{"type": "string", "enum": []string{"once", "interval", "cron"}},
						"schedule_spec": map[string]any{"type": "object"},
						"channel_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
						"enabled":       map[string]any{"type": "boolean"},
						"require_confirm": map[string]any{
							"type":        "boolean",
							"description": "是否需要确认；设为 true 时未给重发参数则沿用原值，原值为空则用默认 300 秒、3 次",
						},
						"confirm_retry_interval_sec": map[string]any{"type": "integer", "minimum": 60, "description": "确认重发间隔（秒）"},
						"confirm_max_retries":        map[string]any{"type": "integer", "minimum": 1, "description": "确认最大重发次数"},
					},
					"required": []string{"id"},
				},
			},
			run: func(ctx context.Context, raw json.RawMessage) (string, error) {
				return s.toolUpdateReminder(ctx, raw)
			},
		},
		{
			spec: llm.Tool{
				Name:        "delete_reminder",
				Description: "提出删除一条提醒。id 来自 list_reminders。",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"id": map[string]any{"type": "integer"}},
					"required":   []string{"id"},
				},
			},
			run: func(ctx context.Context, raw json.RawMessage) (string, error) {
				return s.toolDeleteReminder(ctx, raw)
			},
		},
		{
			spec: llm.Tool{
				Name: "create_channel",
				Description: "提出创建一个通知通道，会生成待确认卡片。config 字段按 type 填写：\n" +
					"- smtp: host, port, username, password_enc（密码原文）, from_addr, to（字符串数组）, security_mode(starttls|implicit_tls|plain)\n" +
					"- dingtalk: webhook_url 与可选 secret_enc\n" +
					"- wecom: webhook_url\n" +
					"- webhook: method(GET|POST), url, 可选 headers/body_template/authorization_enc\n" +
					"- log: 无需字段",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":   map[string]any{"type": "string"},
						"type":   channelTypeSchema,
						"config": map[string]any{"type": "object", "description": "通道配置，字段随 type 变化"},
					},
					"required": []string{"name", "type", "config"},
				},
			},
			run: func(ctx context.Context, raw json.RawMessage) (string, error) {
				return s.toolCreateChannel(ctx, raw)
			},
		},
	}
}

// toolListChannels 返回已配置通道的摘要。
func (s *AIService) toolListChannels(context.Context, json.RawMessage) (string, error) {
	channels, err := s.ChannelSvc.List()
	if err != nil {
		return "", err
	}
	type item struct {
		ID      uint   `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
	}
	out := make([]item, 0, len(channels))
	for _, ch := range channels {
		out = append(out, item{ID: ch.ID, Name: ch.Name, Type: ch.Type, Enabled: ch.Enabled})
	}
	return toJSONString(map[string]any{"channels": out}), nil
}

// toolListReminders 返回提醒摘要。
func (s *AIService) toolListReminders(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Search string `json:"search"`
	}
	_ = json.Unmarshal(raw, &in)
	views, _, err := s.ReminderSvc.List(ListFilter{Search: in.Search, Limit: 20})
	if err != nil {
		return "", err
	}
	type item struct {
		ID                      uint           `json:"id"`
		Title                   string         `json:"title"`
		Content                 string         `json:"content"`
		Calendar                string         `json:"calendar"`
		ScheduleType            string         `json:"schedule_type"`
		ScheduleSpec            map[string]any `json:"schedule_spec"`
		Enabled                 bool           `json:"enabled"`
		ChannelIDs              []uint         `json:"channel_ids"`
		RequireConfirm          bool           `json:"require_confirm"`
		ConfirmRetryIntervalSec int            `json:"confirm_retry_interval_sec"`
		ConfirmMaxRetries       int            `json:"confirm_max_retries"`
		NextFireAt              string         `json:"next_fire_at_local,omitempty"`
	}
	out := make([]item, 0, len(views))
	for _, v := range views {
		out = append(out, item{
			ID:                      v.ID,
			Title:                   v.Title,
			Content:                 truncateRunes(v.Content, 200),
			Calendar:                v.Calendar,
			ScheduleType:            v.ScheduleType,
			ScheduleSpec:            v.ScheduleSpec,
			Enabled:                 v.Enabled,
			ChannelIDs:              v.ChannelIDs,
			RequireConfirm:          v.RequireConfirm,
			ConfirmRetryIntervalSec: v.ConfirmRetryIntervalSec,
			ConfirmMaxRetries:       v.ConfirmMaxRetries,
			NextFireAt:              v.NextFireAtLocal,
		})
	}
	return toJSONString(map[string]any{"reminders": out}), nil
}

// toolCreateReminder 生成一条创建提醒的待确认草稿。
func (s *AIService) toolCreateReminder(_ context.Context, raw json.RawMessage) (string, error) {
	var in ReminderInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if in.Calendar == "" {
		in.Calendar = "solar"
	}
	if in.ContentFormat == "" {
		in.ContentFormat = "text"
	}
	in.Source = "ai"
	if len(in.ChannelIDs) == 0 {
		return "", errors.New("channel_ids 不能为空：请先调用 list_channels 选择通道")
	}
	normalizeConfirmParams(&in)
	// 提前跑一遍服务层校验，让模型拿到明确错误后自行修正，而不是把
	// 无法创建的草稿留给用户。
	if err := s.ReminderSvc.validate(&in); err != nil {
		return "", err
	}
	return s.createAction("create_reminder", "创建提醒："+in.Title+confirmSummarySuffix(in), in)
}

// toolUpdateReminder 生成一条修改提醒的待确认草稿。
//
// 用「现有值 + 提交字段」合并成完整入参：服务层 Update 是全量覆盖语义，
// 直接把模型给的片段传进去会把未提及的字段清空。
func (s *AIService) toolUpdateReminder(_ context.Context, raw json.RawMessage) (string, error) {
	var patch map[string]any
	if err := json.Unmarshal(raw, &patch); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	id, err := jsonInt(patch, "id")
	if err != nil {
		return "", err
	}
	cur, err := s.ReminderSvc.Get(uint(id))
	if err != nil {
		return "", err
	}
	merged := map[string]any{
		"title":                      cur.Title,
		"content":                    cur.Content,
		"content_format":             cur.ContentFormat,
		"calendar":                   cur.Calendar,
		"schedule_type":              cur.ScheduleType,
		"schedule_spec":              cur.ScheduleSpec,
		"channel_ids":                cur.ChannelIDs,
		"require_confirm":            cur.RequireConfirm,
		"confirm_retry_interval_sec": cur.ConfirmRetryIntervalSec,
		"confirm_max_retries":        cur.ConfirmMaxRetries,
		"enabled":                    cur.Enabled,
		"source":                     "ai",
	}
	for k, v := range patch {
		if k == "id" {
			continue
		}
		merged[k] = v
	}
	input := ReminderInput{
		Title:                   asString(merged["title"]),
		Content:                 asString(merged["content"]),
		ContentFormat:           asString(merged["content_format"]),
		Calendar:                asString(merged["calendar"]),
		ScheduleType:            asString(merged["schedule_type"]),
		ScheduleSpec:            asMap(merged["schedule_spec"]),
		ChannelIDs:              asUintSlice(merged["channel_ids"]),
		RequireConfirm:          asBool(merged["require_confirm"]),
		ConfirmRetryIntervalSec: asPositiveInt(merged["confirm_retry_interval_sec"]),
		ConfirmMaxRetries:       asPositiveInt(merged["confirm_max_retries"]),
		Source:                  "ai",
	}
	normalizeConfirmParams(&input)
	if v, ok := merged["enabled"]; ok {
		b, _ := v.(bool)
		input.Enabled = &b
	}
	if err := s.ReminderSvc.validate(&input); err != nil {
		return "", err
	}
	return s.createAction("update_reminder", fmt.Sprintf("修改提醒 #%d：%s", id, input.Title)+confirmSummarySuffix(input),
		map[string]any{"id": id, "input": input})
}

// toolDeleteReminder 生成一条删除提醒的待确认草稿。
func (s *AIService) toolDeleteReminder(_ context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	cur, err := s.ReminderSvc.Get(in.ID)
	if err != nil {
		return "", err
	}
	return s.createAction("delete_reminder", "删除提醒："+cur.Title, map[string]any{"id": in.ID})
}

// toolCreateChannel 生成一条创建通道的待确认草稿。
func (s *AIService) toolCreateChannel(_ context.Context, raw json.RawMessage) (string, error) {
	var in ChannelInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if err := s.ChannelSvc.validateInput(in, false, nil); err != nil {
		return "", err
	}
	return s.createAction("create_channel", "创建通道："+in.Name, in)
}

// createAction 落一条待确认操作，并把 draft_id 告诉模型。
//
// 措辞刻意强调"尚未生效"：如果不说明，模型容易回复"已创建"，
// 用户会以为数据已经落库。
func (s *AIService) createAction(kind, summary string, payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	enc, err := s.Box.EncryptString(string(raw))
	if err != nil {
		return "", err
	}
	act := &models.AIAction{
		ID:         uuid.NewString(),
		Kind:       kind,
		Summary:    truncateRunes(summary, 255),
		PayloadEnc: enc,
		Status:     "pending",
	}
	if err := s.DB.Create(act).Error; err != nil {
		return "", err
	}
	return toJSONString(map[string]any{
		"draft_id": act.ID,
		"status":   "pending",
		"note":     "已生成待确认操作，尚未生效。请告诉用户点击卡片上的「确认执行」后才会真正创建。",
	}), nil
}

// normalizeConfirmParams 为「需要确认」的提醒补齐重发参数。
//
// 模型通常只会传 require_confirm=true，但服务层校验要求间隔 ≥60 秒、次数 ≥1，
// 缺参数会让整个工具调用失败，模型往往改不明白。这里按表单默认值补齐：
// 用户能接受的默认值，胜过一张建不出来的草稿。
func normalizeConfirmParams(in *ReminderInput) {
	if !in.RequireConfirm {
		return
	}
	if in.ConfirmRetryIntervalSec < 60 {
		in.ConfirmRetryIntervalSec = defaultConfirmRetryIntervalSec
	}
	if in.ConfirmMaxRetries < 1 {
		in.ConfirmMaxRetries = defaultConfirmMaxRetries
	}
}

// confirmSummarySuffix 把确认策略拼进草稿摘要，让用户在卡片上就能核对。
func confirmSummarySuffix(in ReminderInput) string {
	if !in.RequireConfirm {
		return ""
	}
	return fmt.Sprintf("（需确认，未确认每 %d 秒重发，共 %d 次）",
		in.ConfirmRetryIntervalSec, in.ConfirmMaxRetries)
}

// jsonInt 从 JSON 对象里取整数。
func jsonInt(m map[string]any, key string) (int64, error) {
	v, ok := m[key]
	if !ok {
		return 0, middleware.NewAppError(middleware.CodeValidationFailed, "缺少参数: "+key).WithField(key)
	}
	switch n := v.(type) {
	case float64:
		return int64(n), nil
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	}
	return 0, middleware.NewAppError(middleware.CodeValidationFailed, "参数类型错误: "+key).WithField(key)
}

// asMap 把任意值规整为 map[string]any。
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// asBool 把任意值规整为 bool。
func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// asUintSlice 把 JSON 数字数组转成 []uint。
func asUintSlice(v any) []uint {
	items, ok := v.([]any)
	if !ok {
		if u, ok := v.([]uint); ok {
			return u
		}
		return nil
	}
	out := make([]uint, 0, len(items))
	for _, item := range items {
		switch n := item.(type) {
		case float64:
			if n > 0 {
				out = append(out, uint(n))
			}
		case int:
			if n > 0 {
				out = append(out, uint(n))
			}
		}
	}
	return out
}

// toJSONString 把任意值序列化为紧凑 JSON 字符串（工具结果）。
func toJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// truncateRunes 按字符截断，避免截出半个汉字；截断时补省略号。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// draftIDOf 从工具结果 JSON 里取出 draft_id；不是写工具的结果时返回空串。
func draftIDOf(out string) string {
	var v struct {
		DraftID string `json:"draft_id"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		return ""
	}
	return v.DraftID
}
