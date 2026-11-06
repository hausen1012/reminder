package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reminder/backend/internal/llm"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// fakeProvider 按预设脚本依次返回响应，避免测试打真实网络。
type fakeProvider struct {
	replies []*llm.Reply
	calls   []llm.Request
	err     error
}

func (f *fakeProvider) Chat(_ context.Context, req llm.Request) (*llm.Reply, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.replies) == 0 {
		return &llm.Reply{Text: "（无脚本）"}, nil
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return r, nil
}

// newAITestService 构造带 fake provider 的 AI 服务，并预置一条 log 通道。
func newAITestService(t *testing.T, replies ...*llm.Reply) (*AIService, *fakeProvider, *gorm.DB) {
	t.Helper()
	db := openTestDB(t)
	chnSvc, _ := newChannelSvcForTestWithDB(t, db)
	remSvc := NewReminderService(db, noopEngine{}, testTZ(cstZone()), NewDispatchService(db, chnSvc, testTZ(cstZone())), nil)
	svc := NewAIService(db, chnSvc.Box, testTZ(cstZone()), remSvc, chnSvc)
	fake := &fakeProvider{replies: replies}
	svc.providerOverride = fake
	// 打开 AI 开关，否则 Chat 直接拒绝
	if err := svc.upsertConfig(KeyAIEnabled, "true"); err != nil {
		t.Fatalf("写入 AI 开关失败: %v", err)
	}
	if _, err := chnSvc.Create(ChannelInput{Name: "测试通道", Type: "log", Config: map[string]any{}}); err != nil {
		t.Fatalf("创建测试通道失败: %v", err)
	}
	return svc, fake, db
}

// toolCall 构造一次工具调用响应。
func toolCall(name string, input string) *llm.Reply {
	return &llm.Reply{
		ToolCalls: []llm.ToolCall{{ID: "call_" + name, Name: name, Input: json.RawMessage(input)}},
	}
}

// chat 是测试里调用 Chat 的简写（无历史）。
func chat(svc *AIService, msg string) (*ChatResult, error) {
	return svc.Chat(context.Background(), ChatInput{Message: msg})
}

// 只回文本时不产生任何待确认操作，也不落任何对话表。
func TestAIChat_TextOnly(t *testing.T) {
	svc, _, db := newAITestService(t, &llm.Reply{Text: "你好，我是助手"})

	res, err := chat(svc, "你好")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if res.Reply != "你好，我是助手" {
		t.Fatalf("reply = %q", res.Reply)
	}
	if len(res.Actions) != 0 {
		t.Fatalf("不应产生待确认操作，实际 %d", len(res.Actions))
	}
	// 对话不入库：库里不应出现任何消息表
	tables := map[string]bool{}
	rows, _ := db.Raw("SELECT name FROM sqlite_master WHERE type='table'").Rows()
	defer rows.Close()
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		tables[name] = true
	}
	if tables["ai_messages"] || tables["ai_conversations"] {
		t.Fatalf("不应创建对话相关表，实际表: %v", tables)
	}
}

// 历史消息由客户端上传，服务端只裁剪不落库。
func TestAIChat_HistoryFromClient(t *testing.T) {
	svc, fake, _ := newAITestService(t, &llm.Reply{Text: "好的"})

	_, err := svc.Chat(context.Background(), ChatInput{
		Message: "第二条",
		History: []ChatMessage{
			{Role: "user", Content: "第一条"},
			{Role: "assistant", Content: "收到"},
			{Role: "bad-role", Content: "应被丢弃"},
			{Role: "user", Content: "   "},
		},
	})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}

	turns := fake.calls[0].Turns
	if len(turns) != 3 {
		t.Fatalf("轮次 = %d，期望 3（两条有效历史 + 本次消息）", len(turns))
	}
	if turns[0].Text != "第一条" || turns[1].Text != "收到" || turns[2].Text != "第二条" {
		t.Fatalf("轮次内容不符: %+v", turns)
	}
}

// 超长历史只保留最近 maxHistoryTurns 条。
func TestAIChat_HistoryTruncated(t *testing.T) {
	svc, fake, _ := newAITestService(t, &llm.Reply{Text: "ok"})

	history := make([]ChatMessage, 0, maxHistoryTurns+10)
	for i := 0; i < maxHistoryTurns+10; i++ {
		history = append(history, ChatMessage{Role: "user", Content: "m" + string(rune('a'+i%26))})
	}
	if _, err := svc.Chat(context.Background(), ChatInput{Message: "最后", History: history}); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if got := len(fake.calls[0].Turns); got != maxHistoryTurns+1 {
		t.Fatalf("轮次 = %d，期望 %d", got, maxHistoryTurns+1)
	}
}

// 创建提醒的工具调用只生成草稿，未确认前库中不应出现提醒。
func TestAIChat_CreateReminderCreatesDraftOnly(t *testing.T) {
	svc, _, db := newAITestService(t,
		toolCall("list_channels", `{}`),
		toolCall("create_reminder", `{
			"title":"喝水",
			"content":"该喝水了",
			"schedule_type":"once",
			"schedule_spec":{"at":"2030-01-01T09:00:00"},
			"channel_ids":[1]
		}`),
		&llm.Reply{Text: "已生成待确认卡片"},
	)

	res, err := chat(svc, "明天九点提醒我喝水")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if len(res.Actions) != 1 {
		t.Fatalf("应生成 1 条待确认操作，实际 %d", len(res.Actions))
	}
	if res.Actions[0].Kind != "create_reminder" || res.Actions[0].Status != "pending" {
		t.Fatalf("操作内容不符: %+v", res.Actions[0])
	}
	if res.Actions[0].PayloadEnc == "" {
		t.Fatal("草稿 payload 必须加密入库")
	}

	var reminders int64
	db.Model(&models.Reminder{}).Count(&reminders)
	if reminders != 0 {
		t.Fatalf("确认前不应创建提醒，实际 %d 条", reminders)
	}
}

// 只回轮本轮的草稿：上一轮生成但未处理的草稿不应重复回传。
func TestAIChat_ReturnsOnlyCurrentRoundActions(t *testing.T) {
	svc, _, db := newAITestService(t,
		toolCall("create_reminder", `{
			"title":"上一轮","schedule_type":"once",
			"schedule_spec":{"at":"2030-01-01T09:00:00"},"channel_ids":[1]
		}`),
		&llm.Reply{Text: "已生成"},
		&llm.Reply{Text: "这轮不建了"},
	)

	first, err := chat(svc, "建第一个")
	if err != nil {
		t.Fatalf("第一次 Chat 失败: %v", err)
	}
	if len(first.Actions) != 1 {
		t.Fatalf("第一轮应生成 1 条草稿，实际 %d", len(first.Actions))
	}

	second, err := chat(svc, "算了")
	if err != nil {
		t.Fatalf("第二次 Chat 失败: %v", err)
	}
	if len(second.Actions) != 0 {
		t.Fatalf("第二轮不应回传旧草稿，实际 %d", len(second.Actions))
	}
	// 旧草稿仍在库里，用户刷新后仍能拿到
	var pending int64
	db.Model(&models.AIAction{}).Where("status = ?", "pending").Count(&pending)
	if pending != 1 {
		t.Fatalf("库里应仍有 1 条待确认草稿，实际 %d", pending)
	}
}

// 确认执行后才真正落库，且状态变为 applied。
func TestAIApplyAction_CreatesReminder(t *testing.T) {
	svc, _, db := newAITestService(t,
		toolCall("create_reminder", `{
			"title":"喝水",
			"schedule_type":"once",
			"schedule_spec":{"at":"2030-01-01T09:00:00"},
			"channel_ids":[1]
		}`),
		&llm.Reply{Text: "已生成待确认卡片"},
	)

	res, err := chat(svc, "提醒我喝水")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	act, err := svc.ApplyAction(res.Actions[0].ID)
	if err != nil {
		t.Fatalf("ApplyAction 失败: %v", err)
	}
	if act.Status != "applied" {
		t.Fatalf("状态 = %q，期望 applied", act.Status)
	}
	if !strings.HasPrefix(act.ResultRef, "reminder:") {
		t.Fatalf("result_ref = %q", act.ResultRef)
	}

	var reminders []models.Reminder
	if err := db.Where("title = ?", "喝水").Find(&reminders).Error; err != nil {
		t.Fatalf("查询提醒失败: %v", err)
	}
	if len(reminders) != 1 {
		t.Fatalf("应创建 1 条提醒，实际 %d", len(reminders))
	}
	// 来源标记为 ai：列表页按来源筛选能区分
	if reminders[0].Source != "ai" {
		t.Fatalf("source = %q，期望 ai", reminders[0].Source)
	}

	// 重复确认必须冲突，避免重复创建
	if _, err := svc.ApplyAction(act.ID); err == nil {
		t.Fatal("重复执行应返回冲突错误")
	}
}

// 创建通道的草稿里可能含密钥，必须加密入库。
func TestAIChat_CreateChannelPayloadEncrypted(t *testing.T) {
	svc, _, db := newAITestService(t,
		toolCall("create_channel", `{
			"name":"钉钉",
			"type":"dingtalk",
			"config":{"webhook_url":"https://example.com/hook","secret_enc":"SEC123456"}
		}`),
		&llm.Reply{Text: "已生成待确认卡片"},
	)

	res, err := chat(svc, "配置钉钉")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if len(res.Actions) != 1 {
		t.Fatalf("应生成 1 条待确认操作，实际 %d", len(res.Actions))
	}
	var stored models.AIAction
	if err := db.First(&stored, "id = ?", res.Actions[0].ID).Error; err != nil {
		t.Fatalf("读取草稿失败: %v", err)
	}
	if strings.Contains(stored.PayloadEnc, "SEC123456") {
		t.Fatal("草稿中不应出现明文密钥")
	}

	// 确认执行后通道落库，且密钥以密文保存
	act, err := svc.ApplyAction(stored.ID)
	if err != nil {
		t.Fatalf("ApplyAction 失败: %v", err)
	}
	if act.Status != "applied" || !strings.HasPrefix(act.ResultRef, "channel:") {
		t.Fatalf("执行结果不符: %+v", act)
	}
}

// 修改提醒时未提供的字段应保持原值（服务层 Update 是全量覆盖语义）。
func TestAIUpdateReminderMergesExistingFields(t *testing.T) {
	svc, _, db := newAITestService(t, &llm.Reply{Text: "ok"})

	created, err := svc.ReminderSvc.Create(ReminderInput{
		Title:        "原标题",
		Content:      "原正文",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: map[string]any{"at": "2030-01-01T09:00:00"},
		ChannelIDs:   []uint{1},
	})
	if err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}

	out, err := svc.toolUpdateReminder(context.Background(), json.RawMessage(`{"id":1,"title":"新标题"}`))
	if err != nil {
		t.Fatalf("toolUpdateReminder 失败: %v", err)
	}
	if !strings.Contains(out, "draft_id") {
		t.Fatalf("应返回 draft_id: %s", out)
	}

	var acts []models.AIAction
	if err := db.Where("status = ?", "pending").Find(&acts).Error; err != nil {
		t.Fatalf("查询草稿失败: %v", err)
	}
	if len(acts) != 1 {
		t.Fatalf("应生成 1 条草稿，实际 %d", len(acts))
	}
	if _, err := svc.ApplyAction(acts[0].ID); err != nil {
		t.Fatalf("ApplyAction 失败: %v", err)
	}

	var got models.Reminder
	if err := db.First(&got, created.ID).Error; err != nil {
		t.Fatalf("查询提醒失败: %v", err)
	}
	if got.Title != "新标题" {
		t.Fatalf("标题 = %q", got.Title)
	}
	if got.Content != "原正文" {
		t.Fatalf("正文被误清空: %q", got.Content)
	}
	var links int64
	db.Model(&models.ReminderChannel{}).Where("reminder_id = ?", got.ID).Count(&links)
	if links != 1 {
		t.Fatalf("通道绑定应保留，实际 %d", links)
	}
}

// 未配置通道时 create_reminder 应报错，让模型自行纠正。
func TestToolCreateReminder_RequiresChannels(t *testing.T) {
	svc, _, _ := newAITestService(t, &llm.Reply{Text: "ok"})
	_, err := svc.toolCreateReminder(context.Background(), json.RawMessage(`{
		"title":"x","schedule_type":"once","schedule_spec":{"at":"2030-01-01T09:00:00"},"channel_ids":[]
	}`))
	if err == nil {
		t.Fatal("channel_ids 为空时应报错")
	}
}

// 工具调用异常应回填成工具结果，让模型下一轮自行修正而不是整轮失败。
func TestAIChat_ToolErrorIsFedBack(t *testing.T) {
	svc, fake, _ := newAITestService(t,
		toolCall("create_reminder", `{"title":""}`),
		&llm.Reply{Text: "标题不能为空，请补充"},
	)
	res, err := chat(svc, "建个提醒")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if res.Reply != "标题不能为空，请补充" {
		t.Fatalf("reply = %q", res.Reply)
	}

	// 第二轮请求里应带上失败的 tool_result
	second := fake.calls[len(fake.calls)-1]
	var sawResult bool
	for _, turn := range second.Turns {
		for _, r := range turn.ToolResults {
			if r.IsError {
				sawResult = true
			}
		}
	}
	if !sawResult {
		t.Fatal("第二轮请求应包含带 is_error 的工具结果")
	}
}

// 工具循环必须有上限：模型一直要求调工具时也要终止。
func TestAIChat_StopsAfterMaxRounds(t *testing.T) {
	replies := make([]*llm.Reply, maxToolRounds+2)
	for i := range replies {
		replies[i] = toolCall("list_channels", `{}`)
	}
	svc, fake, _ := newAITestService(t, replies...)

	res, err := chat(svc, "看看通道")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if len(fake.calls) != maxToolRounds {
		t.Fatalf("调用轮数 = %d，期望 %d", len(fake.calls), maxToolRounds)
	}
	if res.Reply == "" {
		t.Fatal("达到上限也应返回兜底文本")
	}
}

// 未启用时拒绝对话，且提示去哪里开启。
func TestAIChat_Disabled(t *testing.T) {
	svc, _, _ := newAITestService(t, &llm.Reply{Text: "x"})
	if err := svc.upsertConfig(KeyAIEnabled, "false"); err != nil {
		t.Fatalf("写入开关失败: %v", err)
	}
	_, err := chat(svc, "hi")
	if err == nil || !strings.Contains(err.Error(), "未启用") {
		t.Fatalf("应返回未启用错误，实际 %v", err)
	}
}

// 设置读写：密钥回显为 ***，占位符提交时保留原密钥。
func TestAISettings_MaskAndKeepKey(t *testing.T) {
	svc, _, _ := newAITestService(t, &llm.Reply{Text: "x"})

	out, err := svc.SaveSettings(AISettingsInput{
		Enabled:  true,
		Provider: "openai",
		Model:    "deepseek-chat",
		APIKey:   "sk-secret",
	})
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if out.APIKey != "***" {
		t.Fatalf("密钥应脱敏，实际 %q", out.APIKey)
	}

	// 以占位符回传时应保留原密钥
	if _, err := svc.SaveSettings(AISettingsInput{
		Enabled:  true,
		Provider: "openai",
		Model:    "deepseek-chat",
		APIKey:   "***",
	}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	cfg, err := svc.resolveConfig(AISettingsInput{})
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if cfg.APIKey != "sk-secret" {
		t.Fatalf("密钥被误改: %q", cfg.APIKey)
	}

	// 启用但没有密钥时应拒绝
	if err := svc.upsertConfig(KeyAIAPIKey, ""); err != nil {
		t.Fatalf("清空密钥失败: %v", err)
	}
	if _, err := svc.SaveSettings(AISettingsInput{Enabled: true, Provider: "anthropic"}); err == nil {
		t.Fatal("无密钥启用应被拒绝")
	}
}

// 拒绝操作后不能再执行。
func TestAIRejectAction(t *testing.T) {
	svc, _, _ := newAITestService(t,
		toolCall("create_reminder", `{
			"title":"喝水","schedule_type":"once",
			"schedule_spec":{"at":"2030-01-01T09:00:00"},"channel_ids":[1]
		}`),
		&llm.Reply{Text: "已生成"},
	)
	res, err := chat(svc, "提醒")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	act, err := svc.RejectAction(res.Actions[0].ID)
	if err != nil {
		t.Fatalf("RejectAction 失败: %v", err)
	}
	if act.Status != "rejected" {
		t.Fatalf("状态 = %q", act.Status)
	}
	if _, err := svc.ApplyAction(act.ID); err == nil {
		t.Fatal("已拒绝的操作不能再执行")
	}
}

// 草稿跨请求可见：刷新页面后仍能拉到待确认列表。
func TestAIListActions(t *testing.T) {
	svc, _, _ := newAITestService(t,
		toolCall("create_reminder", `{
			"title":"喝水","schedule_type":"once",
			"schedule_spec":{"at":"2030-01-01T09:00:00"},"channel_ids":[1]
		}`),
		&llm.Reply{Text: "已生成"},
	)
	if _, err := chat(svc, "提醒"); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}

	pending, err := svc.ListActions("pending")
	if err != nil {
		t.Fatalf("ListActions 失败: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("待确认 = %d，期望 1", len(pending))
	}

	// 已执行的不在 pending 里
	if _, err := svc.ApplyAction(pending[0].ID); err != nil {
		t.Fatalf("ApplyAction 失败: %v", err)
	}
	pending, _ = svc.ListActions("pending")
	if len(pending) != 0 {
		t.Fatalf("执行后 pending 应为空，实际 %d", len(pending))
	}
	all, _ := svc.ListActions("")
	if len(all) != 1 {
		t.Fatalf("全部草稿 = %d，期望 1", len(all))
	}
}

// 空消息与超长消息都应被拒绝。
func TestAIChat_ValidatesMessage(t *testing.T) {
	svc, _, _ := newAITestService(t, &llm.Reply{Text: "x"})
	if _, err := chat(svc, "   "); err == nil {
		t.Fatal("空消息应报错")
	}
	if _, err := chat(svc, strings.Repeat("啊", 4001)); err == nil {
		t.Fatal("超长消息应报错")
	}
}

// Provider 报错时应转成可读错误而不是 500 原文。
func TestAIChat_ProviderError(t *testing.T) {
	svc, fake, _ := newAITestService(t)
	fake.err = errors.New("connection refused")
	_, err := chat(svc, "hi")
	if err == nil || !strings.Contains(err.Error(), "AI 调用失败") {
		t.Fatalf("应返回 AI 调用失败，实际 %v", err)
	}
}

// 工具列表必须覆盖全部写操作，且 schema 都带 properties。
func TestAIToolDefinitions(t *testing.T) {
	svc, _, _ := newAITestService(t, &llm.Reply{Text: "x"})
	defs := svc.toolDefinitions()
	want := map[string]bool{
		"list_channels": true, "list_reminders": true, "create_reminder": true,
		"update_reminder": true, "delete_reminder": true, "create_channel": true,
	}
	for _, d := range defs {
		delete(want, d.spec.Name)
		if d.spec.Name == "" || d.spec.Description == "" {
			t.Fatalf("工具缺少名称或描述: %+v", d.spec)
		}
		if d.spec.InputSchema["type"] != "object" {
			t.Fatalf("工具 %s 的 schema 必须是 object", d.spec.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("缺少工具: %v", want)
	}
}

// 自定义提示词：保存后追加在内置规则之后，随对话请求发出；留空则只用内置提示词。
func TestAICustomPrompt(t *testing.T) {
	svc, fake, _ := newAITestService(t, &llm.Reply{Text: "ok"})

	// 未配置时，系统提示里不应出现追加段落
	if _, err := chat(svc, "hi"); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if strings.Contains(fake.calls[0].System, "额外要求") {
		t.Fatalf("未配置自定义提示词时不应出现追加段落: %s", fake.calls[0].System)
	}

	saved, err := svc.SaveSettings(AISettingsInput{
		Enabled:      true,
		Provider:     "anthropic",
		APIKey:       "sk-x",
		CustomPrompt: "  称呼我为老板  ",
	})
	if err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	if saved.CustomPrompt != "称呼我为老板" {
		t.Fatalf("custom_prompt 应去除首尾空白，实际 %q", saved.CustomPrompt)
	}

	if _, err := chat(svc, "hi"); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	system := fake.calls[len(fake.calls)-1].System
	idxBase := strings.Index(system, "你是 Reminder 自托管提醒系统的助手")
	idxCustom := strings.Index(system, "称呼我为老板")
	if idxBase < 0 || idxCustom < 0 {
		t.Fatalf("系统提示应同时包含内置规则与自定义内容: %s", system)
	}
	if idxCustom < idxBase {
		t.Fatalf("自定义内容应追加在内置规则之后: %s", system)
	}
	// 内置的待确认规则不能被自定义内容挤掉
	if !strings.Contains(system, "待确认操作") {
		t.Fatalf("内置规则不应丢失: %s", system)
	}

	// 清空后不再追加
	if _, err := svc.SaveSettings(AISettingsInput{
		Enabled: true, Provider: "anthropic", APIKey: "***", CustomPrompt: "",
	}); err != nil {
		t.Fatalf("清空提示词失败: %v", err)
	}
	if got, err := svc.SystemPrompt(); err != nil {
		t.Fatalf("读取系统提示失败: %v", err)
	} else if strings.Contains(got, "称呼我为老板") {
		t.Fatalf("清空后不应再包含自定义内容: %s", got)
	}
}

// 自定义提示词超长应被拒绝。
func TestAISaveSettings_CustomPromptTooLong(t *testing.T) {
	svc, _, _ := newAITestService(t, &llm.Reply{Text: "ok"})
	_, err := svc.SaveSettings(AISettingsInput{
		Provider:     "anthropic",
		APIKey:       "sk-x",
		CustomPrompt: strings.Repeat("啊", maxCustomPromptRunes+1),
	})
	if err == nil || !strings.Contains(err.Error(), "过长") {
		t.Fatalf("应返回过长错误，实际 %v", err)
	}
}

// AI 输出上限与重试次数：默认值、读写回显与非法值校验。
func TestAISettingsMaxTokensAndRetries(t *testing.T) {
	svc, _, _ := newAITestService(t, &llm.Reply{Text: "x"})

	// 未配置时读回默认值
	out, err := svc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings 失败: %v", err)
	}
	if out.MaxTokens != defaultMaxTokens {
		t.Fatalf("默认 max_tokens = %d，期望 %d", out.MaxTokens, defaultMaxTokens)
	}
	if out.MaxRetries != defaultMaxRetries {
		t.Fatalf("默认 max_retries = %d，期望 %d", out.MaxRetries, defaultMaxRetries)
	}

	// 保存后读回
	three := 3
	out, err = svc.SaveSettings(AISettingsInput{
		Provider:   "openai",
		Model:      "deepseek-chat",
		APIKey:     "sk-x",
		MaxTokens:  8192,
		MaxRetries: &three,
	})
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if out.MaxTokens != 8192 || out.MaxRetries != 3 {
		t.Fatalf("读回值不符: max_tokens=%d max_retries=%d", out.MaxTokens, out.MaxRetries)
	}

	// 解析进运行时配置，Chat 会用它构造请求
	cfg, err := svc.resolveConfig(AISettingsInput{})
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if cfg.MaxTokens != 8192 || cfg.MaxRetries != 3 {
		t.Fatalf("运行时配置不符: %+v", cfg)
	}

	// 越界的 token 上限与重试次数应被拒绝
	if _, err := svc.SaveSettings(AISettingsInput{Provider: "openai", APIKey: "sk-x", MaxTokens: minMaxTokens - 1}); err == nil {
		t.Fatal("过小的 max_tokens 应被拒绝")
	}
	if _, err := svc.SaveSettings(AISettingsInput{Provider: "openai", APIKey: "sk-x", MaxTokens: maxMaxTokens + 1}); err == nil {
		t.Fatal("过大的 max_tokens 应被拒绝")
	}
	minusOne := -1
	if _, err := svc.SaveSettings(AISettingsInput{Provider: "openai", APIKey: "sk-x", MaxRetries: &minusOne}); err == nil {
		t.Fatal("负数 max_retries 应被拒绝")
	}
	tooMany := maxMaxRetries + 1
	if _, err := svc.SaveSettings(AISettingsInput{Provider: "openai", APIKey: "sk-x", MaxRetries: &tooMany}); err == nil {
		t.Fatal("过大的 max_retries 应被拒绝")
	}

	// 显式传 0 是合法设置：表示不重试
	zero := 0
	out, err = svc.SaveSettings(AISettingsInput{Provider: "openai", APIKey: "sk-x", MaxRetries: &zero})
	if err != nil {
		t.Fatalf("显式 0 次重试应被接受: %v", err)
	}
	if out.MaxRetries != 0 {
		t.Fatalf("显式 0 次重试读回 = %d", out.MaxRetries)
	}
	// 但库里 0 与「未配置」区分得开：重读设置仍为 0，而空串回退默认 10
	if out, _ = svc.GetSettings(); out.MaxRetries != 0 {
		t.Fatalf("库里存 0 后读回 = %d", out.MaxRetries)
	}
	if err := svc.upsertConfig(KeyAIMaxRetries, ""); err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	if out, _ = svc.GetSettings(); out.MaxRetries != defaultMaxRetries {
		t.Fatalf("未配置时应回退默认 %d，实际 %d", defaultMaxRetries, out.MaxRetries)
	}
}

// 配置表里存了脏值时回退到默认值，而不是把 0 传给厂商。
func TestAIParseSettingsFallsBackOnGarbage(t *testing.T) {
	svc, _, _ := newAITestService(t, &llm.Reply{Text: "x"})
	if err := svc.upsertConfig(KeyAIMaxTokens, "abc"); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := svc.upsertConfig(KeyAIMaxRetries, "-5"); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	out, err := svc.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings 失败: %v", err)
	}
	if out.MaxTokens != defaultMaxTokens || out.MaxRetries != defaultMaxRetries {
		t.Fatalf("脏值应回退默认: max_tokens=%d max_retries=%d", out.MaxTokens, out.MaxRetries)
	}
}

// 配置的输出上限要真正传给 Provider 的请求。
func TestAIChatPassesMaxTokens(t *testing.T) {
	svc, fake, _ := newAITestService(t, &llm.Reply{Text: "ok"})
	if _, err := svc.SaveSettings(AISettingsInput{
		Enabled: true, Provider: "openai", Model: "deepseek-chat", APIKey: "sk-x", MaxTokens: 1234,
	}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	if _, err := chat(svc, "hi"); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if got := fake.calls[0].MaxTokens; got != 1234 {
		t.Fatalf("MaxTokens = %d，期望 1234", got)
	}
}

// 模型输出达到上限时停止本轮，不再执行被截断的工具调用。
func TestAIChat_StopsWhenReplyTruncated(t *testing.T) {
	svc, fake, db := newAITestService(t,
		&llm.Reply{
			Text:      "我要创建提醒",
			ToolCalls: []llm.ToolCall{{ID: "c1", Name: "create_reminder", Input: json.RawMessage(`{"title":"喝水"`)}},
			Truncated: true,
		},
	)

	res, err := chat(svc, "提醒我喝水")
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("截断后不应继续请求，实际 %d 次", len(fake.calls))
	}
	if !strings.Contains(res.Reply, "最大上下文窗口") {
		t.Fatalf("应提示已停止，实际 %q", res.Reply)
	}
	if len(res.Actions) != 0 {
		t.Fatalf("截断的工具调用不应生成草稿，实际 %d 条", len(res.Actions))
	}
	var count int64
	db.Model(&models.AIAction{}).Count(&count)
	if count != 0 {
		t.Fatalf("库里不应有草稿，实际 %d 条", count)
	}
}

// 重试事件要透传给 sink，且结果里的 retries 计数与实际重试次数一致。
func TestAIChatStream_EmitsRetryEvents(t *testing.T) {
	// 只失败一次，真实的 200ms 退避可以接受
	svc, _, _ := newAITestService(t)
	svc.providerOverride = &flakyProvider{failTimes: 1}

	var events []string
	var retries []ChatRetry
	res, err := svc.ChatStream(context.Background(), ChatInput{Message: "你好"}, func(event string, data any) {
		events = append(events, event)
		if r, ok := data.(ChatRetry); ok {
			retries = append(retries, r)
		}
	})
	if err != nil {
		t.Fatalf("ChatStream 失败: %v", err)
	}
	if res.Retries != 1 {
		t.Fatalf("retries = %d，期望 1", res.Retries)
	}
	if len(retries) != 1 || retries[0].Attempt != 1 || retries[0].Max != defaultMaxRetries {
		t.Fatalf("重试事件不符: %+v", retries)
	}
	if retries[0].Reason == "" {
		t.Fatal("重试事件应带原因")
	}
	if !containsString(events, ChatEventStatus) {
		t.Fatalf("应推 thinking 状态事件: %v", events)
	}
}

// flakyProvider 前 failTimes 次返回可重试错误，之后返回文本。
type flakyProvider struct {
	failTimes int
	calls     int
}

func (p *flakyProvider) Chat(_ context.Context, _ llm.Request) (*llm.Reply, error) {
	p.calls++
	if p.calls <= p.failTimes {
		return nil, &llm.HTTPStatusError{Status: 429, Message: "rate limited"}
	}
	return &llm.Reply{Text: "好了"}, nil
}

// containsString 判断切片里是否有某字符串。
func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// PurgeActions 只删超过保留期的草稿，保留期内的（无论状态）都不动。
func TestAIPurgeActions(t *testing.T) {
	svc, _, db := newAITestService(t, &llm.Reply{Text: "ok"})

	now := time.Now()
	mk := func(status string, age time.Duration) *models.AIAction {
		act := &models.AIAction{
			ID:         uuid.NewString(),
			Kind:       "create_reminder",
			Summary:    "x",
			PayloadEnc: "x",
			Status:     status,
			CreatedAt:  models.TimeOf(now.Add(-age)),
			UpdatedAt:  models.TimeOf(now.Add(-age)),
		}
		if err := db.Create(act).Error; err != nil {
			t.Fatalf("插入草稿失败: %v", err)
		}
		return act
	}

	oldPending := mk("pending", 91*24*time.Hour)
	oldApplied := mk("applied", 100*24*time.Hour)
	recentApplied := mk("applied", 89*24*time.Hour)
	recentPending := mk("pending", time.Hour)

	n, err := svc.PurgeActions(ActionRetention)
	if err != nil {
		t.Fatalf("PurgeActions 失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("删除数 = %d，期望 2（超期的 pending 与 applied 各一条）", n)
	}

	var ids []string
	if err := db.Model(&models.AIAction{}).Pluck("id", &ids).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	alive := map[string]bool{}
	for _, id := range ids {
		alive[id] = true
	}
	if alive[oldPending.ID] || alive[oldApplied.ID] {
		t.Fatal("超期草稿应被删除")
	}
	if !alive[recentApplied.ID] || !alive[recentPending.ID] {
		t.Fatal("保留期内草稿不应被删除")
	}
}

// 保留期非正数时不做任何事（避免误调用清空全表）。
func TestAIPurgeActionsNonPositive(t *testing.T) {
	svc, _, db := newAITestService(t, &llm.Reply{Text: "ok"})
	if err := db.Create(&models.AIAction{
		ID: "a1", Kind: "create_reminder", Summary: "x", PayloadEnc: "x", Status: "applied",
		CreatedAt: models.TimeOf(time.Now().Add(-1000 * time.Hour)),
	}).Error; err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	n, err := svc.PurgeActions(0)
	if err != nil || n != 0 {
		t.Fatalf("非正保留期应不删除，实际 n=%d err=%v", n, err)
	}
	var count int64
	db.Model(&models.AIAction{}).Count(&count)
	if count != 1 {
		t.Fatalf("草稿不应被删除，实际剩 %d", count)
	}
}

type errorOnRoundProvider struct {
	round int
	errOn int
}

func (p *errorOnRoundProvider) Chat(_ context.Context, _ llm.Request) (*llm.Reply, error) {
	p.round++
	if p.round == p.errOn {
		return nil, errors.New("HTTP 429: rps exhausted")
	}
	return toolCall("create_reminder", `{
		"title":"喝水",
		"schedule_type":"once",
		"schedule_spec":{"at":"2030-01-01T09:00:00"},
		"channel_ids":[1]
	}`), nil
}

// 验证：工具调用入库草稿后，后续轮次模型调用出错时，已入库的草稿会被自动回滚清理，避免残留孤儿待确认操作。
func TestAIChat_CleansUpDraftsOnSubsequentRoundError(t *testing.T) {
	svc, _, db := newAITestService(t)
	svc.providerOverride = &errorOnRoundProvider{errOn: 2}

	_, err := chat(svc, "提醒我喝水")
	if err == nil {
		t.Fatal("期望返回错误，但成功了")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("期望包含 429 错误，实际: %v", err)
	}

	var count int64
	db.Model(&models.AIAction{}).Count(&count)
	if count != 0 {
		t.Fatalf("后续轮次报错后草稿必须被回滚清理，实际残留 %d 条", count)
	}
}

// 验证：并发执行 ApplyAction 时，原子 CAS 保证仅有一次成功，避免生成重复实体。
func TestAIApplyAction_ConcurrentExecution(t *testing.T) {
	svc, _, db := newAITestService(t, &llm.Reply{Text: "ok"})

	payload, _ := json.Marshal(ReminderInput{
		Title:        "并发测试提醒",
		Content:      "并发正文",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: map[string]any{"at": "2030-01-01T09:00:00"},
		ChannelIDs:   []uint{1},
	})
	enc, err := svc.Box.EncryptString(string(payload))
	if err != nil {
		t.Fatalf("加密草稿失败: %v", err)
	}

	act := &models.AIAction{
		ID:         "act_concurrent_test",
		Kind:       "create_reminder",
		Summary:    "创建并发测试提醒",
		PayloadEnc: enc,
		Status:     "pending",
	}
	if err := db.Create(act).Error; err != nil {
		t.Fatalf("创建草稿失败: %v", err)
	}

	const n = 5
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.ApplyAction(act.ID)
		}(i)
	}
	wg.Wait()

	successCount := 0
	conflictCount := 0
	for _, e := range errs {
		if e == nil {
			successCount++
		} else {
			var appErr *middleware.AppError
			if errors.As(e, &appErr) && appErr.Code == middleware.CodeConflict {
				conflictCount++
			}
		}
	}

	if successCount != 1 {
		t.Fatalf("期望仅有 1 次执行成功，实际成功 %d 次: %v", successCount, errs)
	}
	if conflictCount != n-1 {
		t.Fatalf("期望 %d 次报 CodeConflict 冲突，实际 %d 次", n-1, conflictCount)
	}

	var reminderCount int64
	db.Model(&models.Reminder{}).Where("title = ?", "并发测试提醒").Count(&reminderCount)
	if reminderCount != 1 {
		t.Fatalf("期望数据库中仅创建 1 条提醒，实际创建了 %d 条", reminderCount)
	}
}
