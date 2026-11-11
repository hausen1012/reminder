package services

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/notifier"
	"github.com/reminder/backend/internal/testutil"
	"gorm.io/gorm"
)

// createChannel 创建一个测试用通道。
func createChannel(t *testing.T, db *gorm.DB, chType string) *models.Channel {
	t.Helper()
	ch := &models.Channel{
		Name:   testutil.UniqueName(chType),
		Type:   chType,
		Config: []byte("{}"),
	}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}
	return ch
}

// bindChannels 为提醒绑定通道。
func bindChannels(t *testing.T, db *gorm.DB, reminderID uint, channelIDs ...uint) {
	t.Helper()
	for _, id := range channelIDs {
		if err := db.Create(&models.ReminderChannel{ReminderID: reminderID, ChannelID: id}).Error; err != nil {
			t.Fatalf("绑定通道失败: %v", err)
		}
	}
}

// createPendingLog 创建一条 pending 状态的投递日志。
func createPendingLog(t *testing.T, db *gorm.DB, r *models.Reminder) uint {
	t.Helper()
	dl := &models.DeliveryLog{
		ReminderID: r.ID,
		FiredAt:    models.TimeNow(),
		Title:      r.Title,
		Content:    r.Content,
		Status:     "pending",
		Source:     r.Source,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	return dl.ID
}

// newDispatchStack 构造只含 dispatch 的最小栈（无确认机制）。
func newDispatchStack(t *testing.T, db *gorm.DB) *DispatchService {
	t.Helper()
	loc := time.FixedZone("CST", 8*3600)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	dispatch := NewDispatchService(db, NewChannelService(db, box, testTZ(loc)), testTZ(loc))
	dispatch.RetryDelays = []time.Duration{0} // 测试中不等待
	return dispatch
}

// 没有绑定任何通道时，日志应被标记为 failed。
func TestDispatchRun_NoChannelsMarksFailed(t *testing.T) {
	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	r := &models.Reminder{
		Title:        "无通道",
		Content:      "正文",
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	var got models.DeliveryLog
	if err := db.First(&got, logID).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("状态 = %q，期望 failed", got.Status)
	}
}

// 所有通道都成功时，日志标记 success，并写入对应条数的 attempt。
func TestDispatchRun_AllChannelsSucceed(t *testing.T) {
	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	ch1 := createChannel(t, db, "mock")
	ch2 := createChannel(t, db, "mock")

	r := &models.Reminder{
		Title:        "全成功",
		Content:      "正文 {{now_date}}",
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, ch1.ID, ch2.ID)
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	var got models.DeliveryLog
	if err := db.First(&got, logID).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if got.Status != "success" {
		t.Fatalf("状态 = %q，期望 success", got.Status)
	}

	var attempts []models.DeliveryAttempt
	if err := db.Where("delivery_log_id = ?", logID).Find(&attempts).Error; err != nil {
		t.Fatalf("查询 attempt 失败: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("期望 2 条 attempt，实际 %d", len(attempts))
	}
	for _, a := range attempts {
		if a.Status != "success" {
			t.Fatalf("attempt 状态 = %q", a.Status)
		}
	}
	// 渲染后的内容应写入日志快照，{{now_date}} 已被替换
	if got.Content == r.Content {
		t.Fatalf("日志内容未渲染，仍是模板: %q", got.Content)
	}
}

// 部分通道失败时，日志应标记为 partial。
func TestDispatchRun_PartialWhenSomeFailed(t *testing.T) {
	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	okCh := createChannel(t, db, "mock")
	// 不支持的通道类型会在 Get(type) 阶段失败，模拟一个必定失败的通道
	badCh := createChannel(t, db, "unknown-type")

	r := &models.Reminder{
		Title:        "部分失败",
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, okCh.ID, badCh.ID)
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	var got models.DeliveryLog
	if err := db.First(&got, logID).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if got.Status != "partial" {
		t.Fatalf("状态 = %q，期望 partial", got.Status)
	}
}

// 全部通道失败时，日志应标记为 failed。
func TestDispatchRun_AllFailed(t *testing.T) {
	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	badCh := createChannel(t, db, "unknown-type")

	r := &models.Reminder{
		Title:        "全失败",
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, badCh.ID)
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	var got models.DeliveryLog
	if err := db.First(&got, logID).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("状态 = %q，期望 failed", got.Status)
	}

	var attempts []models.DeliveryAttempt
	db.Where("delivery_log_id = ?", logID).Find(&attempts)
	if len(attempts) == 0 {
		t.Fatal("失败时应写入 attempt 记录")
	}
	if attempts[0].Error == "" {
		t.Fatal("失败 attempt 应记录错误信息")
	}
}

// 空参数应直接返回，不 panic。
func TestDispatchRun_NilOrZeroArgs(t *testing.T) {
	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	dispatch.Run(context.Background(), nil, 1)
	dispatch.Run(context.Background(), &models.Reminder{}, 0)
}

// newDispatchForDryRun 构造仅用于试发的 dispatch 服务。
func newDispatchForDryRun(t *testing.T, db *gorm.DB) *DispatchService {
	t.Helper()
	loc := time.FixedZone("CST", 8*3600)
	d := NewDispatchService(db, &ChannelService{DB: db, TZ: testTZ(loc)}, testTZ(loc))
	d.RetryDelays = []time.Duration{0}
	return d
}

// DryRun 全成功时不返回错误，且不落任何日志/attempt。
func TestDispatchDryRun_AllSucceedNoRecords(t *testing.T) {
	db := openTestDB(t)
	d := newDispatchForDryRun(t, db)

	ch1 := createChannel(t, db, "mock")
	ch2 := createChannel(t, db, "mock")

	msg := buildRenderedMessage("标题", "正文", "text", map[string]string{"title": "标题", "content": "正文"})
	if err := d.DryRun(t.Context(), []*models.Channel{ch1, ch2}, msg); err != nil {
		t.Fatalf("DryRun 应成功: %v", err)
	}

	// 试发不应写任何记录
	var logs, attempts int64
	db.Model(&models.DeliveryLog{}).Count(&logs)
	db.Model(&models.DeliveryAttempt{}).Count(&attempts)
	if logs != 0 || attempts != 0 {
		t.Fatalf("试发不应落库，实际 logs=%d attempts=%d", logs, attempts)
	}
}

// DryRun 遇到未知通道类型时应返回错误，且错误信息包含通道信息。
func TestDispatchDryRun_UnknownTypeFails(t *testing.T) {
	db := openTestDB(t)
	d := newDispatchForDryRun(t, db)

	bad := &models.Channel{Name: "bad", Type: "unknown", Config: []byte("{}")}
	err := d.DryRun(t.Context(), []*models.Channel{bad}, buildRenderedMessage("t", "c", "text", map[string]string{}))
	if err == nil {
		t.Fatal("未知类型应报错")
	}
	if !containsSub(err.Error(), "bad") {
		t.Fatalf("错误信息应包含通道名: %v", err)
	}
}

// DryRun 空通道列表应直接成功。
func TestDispatchDryRun_Empty(t *testing.T) {
	db := openTestDB(t)
	d := newDispatchForDryRun(t, db)

	if err := d.DryRun(t.Context(), nil, buildRenderedMessage("t", "c", "text", map[string]string{})); err != nil {
		t.Fatalf("空通道应成功: %v", err)
	}
}

// DryRun 的 ctx 取消时应尽快返回。
func TestDispatchDryRun_ContextCancelled(t *testing.T) {
	db := openTestDB(t)
	d := newDispatchForDryRun(t, db)
	d.RetryDelays = []time.Duration{0, time.Hour} // 第二次重试要等很久

	// 用一个必定失败的通道类型触发重试分支
	bad := &models.Channel{Name: "x", Type: "unknown", Config: []byte("{}")}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.DryRun(ctx, []*models.Channel{bad}, buildRenderedMessage("t", "c", "text", map[string]string{}))
	}()

	select {
	case <-done:
		// 已返回
	case <-time.After(5 * time.Second):
		t.Fatal("ctx 取消后 DryRun 未及时返回")
	}
}

// 渲染后的消息里 {{now_date}} 应被替换，且 content 变量被回写。
func TestBuildRenderedMessage(t *testing.T) {
	vars := map[string]string{"title": "标题", "content": "正文", "now_date": "2026-01-01"}
	msg := buildRenderedMessage("{{title}}", "今天是 {{now_date}}", "text", vars)

	if msg.Subject != "标题" {
		t.Fatalf("subject = %q", msg.Subject)
	}
	if msg.Body != "今天是 2026-01-01" {
		t.Fatalf("body = %q", msg.Body)
	}
	// vars["content"] 应被回写为渲染后的 body
	if vars["content"] != msg.Body {
		t.Fatalf("vars[content] = %q，应回写为渲染结果", vars["content"])
	}
	if msg.Format != "text" {
		t.Fatalf("format = %q", msg.Format)
	}
}

// 标题里的占位符渲染后也应回写 vars["title"]，供 Webhook 模板等下游使用。
func TestBuildRenderedMessage_WritesBackTitle(t *testing.T) {
	vars := map[string]string{"title": "原标题", "now_date": "2026-01-01"}
	msg := buildRenderedMessage("标题 {{now_date}}", "正文", "text", vars)

	if msg.Subject != "标题 2026-01-01" {
		t.Fatalf("subject = %q", msg.Subject)
	}
	if vars["title"] != msg.Subject {
		t.Fatalf("vars[title] = %q，应回写为渲染结果", vars["title"])
	}
}

// Webhook 模板里的 {{title}} 应拿到展开后的标题。
func TestDispatchWebhookTemplateGetsRenderedTitle(t *testing.T) {
	var gotBody map[string]string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	cfg, _ := json.Marshal(map[string]any{
		"method":        "POST",
		"url":           sink.URL,
		"body_template": `{"title":"{{title}}","content":"{{content}}"}`,
	})
	ch := &models.Channel{Name: testutil.UniqueName("webhook"), Type: "webhook", Config: cfg}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	r := &models.Reminder{
		Title:        "标题 {{now_date}}",
		Content:      "正文",
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, ch.ID)

	dispatch.Run(context.Background(), r, createPendingLog(t, db, r))

	if strings.Contains(gotBody["title"], "{{") {
		t.Fatalf("Webhook 的 {{title}} 仍有残留占位符: %q", gotBody["title"])
	}
	if !strings.HasPrefix(gotBody["title"], "标题 ") {
		t.Fatalf("Webhook 的 {{title}} 不是渲染后的标题: %q", gotBody["title"])
	}
}

// 正文写带空格的 {{ confirm_url }} 时，渲染结果已含链接，不得再自动追加一条。
func TestDispatchDoesNotAppendConfirmURLTwice(t *testing.T) {
	var gotBody map[string]string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	cfg, _ := json.Marshal(map[string]any{
		"method":        "POST",
		"url":           sink.URL,
		"body_template": `{"content":"{{content}}"}`,
	})
	ch := &models.Channel{Name: testutil.UniqueName("webhook"), Type: "webhook", Config: cfg}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}
	r := &models.Reminder{
		Title:                   "确认",
		Content:                 "请点击 {{ confirm_url }} 完成确认",
		Calendar:                "solar",
		ScheduleType:            "once",
		Enabled:                 true,
		RequireConfirm:          true,
		ConfirmRetryIntervalSec: 60,
		ConfirmMaxRetries:       1,
		Source:                  "web",
		NextFireAt:              models.TimePtrOf(time.Now().Add(time.Hour)),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, ch.ID)

	stack.dispatch.Run(context.Background(), r, createPendingLog(t, db, r))

	if n := strings.Count(gotBody["content"], "/c/"); n != 1 {
		t.Fatalf("确认链接出现 %d 次，应只有 1 次:\n%s", n, gotBody["content"])
	}
}

// buildVars 应包含设计文档列出的全部内置变量。
func TestBuildVars(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	planned := time.Date(2026, 6, 5, 9, 0, 0, 0, loc)
	fired := planned.Add(2 * time.Second)

	r := &models.Reminder{
		Title: "标题", Content: "正文", Source: "web", ID: 42,
	}
	vars := buildVars(r, fired, planned, loc)

	required := []string{
		"now", "now_date", "now_time", "fire_at",
		"trigger_date", "trigger_time", "weekday",
		"title", "content", "lunar_date", "reminder_id", "reminder_source",
	}
	for _, k := range required {
		if _, ok := vars[k]; !ok {
			t.Fatalf("缺少内置变量 %q", k)
		}
	}

	if vars["title"] != "标题" || vars["content"] != "正文" {
		t.Fatalf("title/content 不正确: %v", vars)
	}
	if vars["reminder_id"] != "42" {
		t.Fatalf("reminder_id = %q", vars["reminder_id"])
	}
	if vars["fire_at"] != "2026-06-05 09:00:00" {
		t.Fatalf("fire_at = %q", vars["fire_at"])
	}
	// weekday 应为中文
	if vars["weekday"] != "周五" {
		t.Fatalf("weekday = %q，期望 周五", vars["weekday"])
	}
	// lunar_date 应含"农历"，年份为中文数字
	if !containsSub(vars["lunar_date"], "农历") {
		t.Fatalf("lunar_date = %q", vars["lunar_date"])
	}
	if vars["lunar_date"] != "农历二〇二六年四月二十" {
		t.Fatalf("lunar_date = %q，期望中文数字纪年", vars["lunar_date"])
	}
}

// buildVars 传 nil 时区应回退到 Local，不 panic。
func TestBuildVars_NilLocation(t *testing.T) {
	now := time.Now()
	r := &models.Reminder{Title: "t"}
	vars := buildVars(r, now, now, nil)
	if vars["now"] == "" {
		t.Fatal("nil 时区时应回退到 Local")
	}
}

// captureLogs 把全局 slog 临时重定向到内存 buffer，t.Cleanup 时还原。
//
// slog.SetDefault 是进程级全局状态，因此本文件里的用例都不要标记 t.Parallel。
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// 投递日志不能泄露确认链接。
//
// vars 里带 confirm_url，拼出来就是完整可用的确认令牌；
// 谁读到日志谁就能替用户把提醒确认掉，因此绝不能出现在日志里。
// 这里刻意不用 log 通道——它的职责就是把正文打进日志，会掩盖要验证的点。
func TestDispatchLogDoesNotLeakConfirmURL(t *testing.T) {
	logs := captureLogs(t)

	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)
	reminder := createConfirmReminderWithWebhook(t, db, 60, 1, sink.URL)

	if _, err := triggerDispatch(context.Background(), stack.dispatch, reminder); err != nil {
		t.Fatalf("触发投递失败: %v", err)
	}

	var tok models.ConfirmToken
	if err := db.First(&tok).Error; err != nil {
		t.Fatalf("确认令牌未创建: %v", err)
	}
	if len(tok.Token) < 32 {
		t.Fatalf("令牌长度异常: %q", tok.Token)
	}

	out := logs.String()
	if strings.Contains(out, tok.Token) {
		t.Fatalf("日志里出现了确认令牌明文:\n%s", out)
	}
	// 变量数量这类"形状信息"仍应保留，便于排查
	if !strings.Contains(out, "vars_count") {
		t.Fatalf("日志缺少 vars_count 字段，排查信息丢失:\n%s", out)
	}
}

// webhook 通道的日志不能带 Authorization 等 header 取值。
func TestWebhookLogRedactsCredentials(t *testing.T) {
	logs := captureLogs(t)

	const secretValue = "super-secret-token-value"
	var gotAuth string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	// WebhookConfig.AuthorizationEnc 在传入 Notifier 时已是明文
	// （ChannelService 负责落库加密与读出解密），这里直接给明文。
	cfg, _ := json.Marshal(map[string]any{
		"method":            "POST",
		"url":               sink.URL + "/hook?access_token=query-secret",
		"authorization_enc": "Bearer " + secretValue,
	})

	n, err := notifier.Get("webhook")
	if err != nil {
		t.Fatalf("获取 webhook 发送器失败: %v", err)
	}
	if err := n.Send(context.Background(), cfg, notifier.Message{Subject: "s", Body: "b"}); err != nil {
		t.Fatalf("webhook 发送失败: %v", err)
	}

	// 凭据必须真的发出去（功能没被削弱）
	if gotAuth != "Bearer "+secretValue {
		t.Fatalf("Authorization 未正确发送: %q", gotAuth)
	}

	out := logs.String()
	for _, leak := range []string{secretValue, "query-secret"} {
		if strings.Contains(out, leak) {
			t.Fatalf("日志里出现了凭据 %q:\n%s", leak, out)
		}
	}
}

// createConfirmReminderWithWebhook 用 webhook 通道创建"需要确认"的提醒。
func createConfirmReminderWithWebhook(t *testing.T, db *gorm.DB, intervalSec, maxRetries int, url string) *models.Reminder {
	t.Helper()
	return createConfirmReminderWithWebhookContent(t, db, intervalSec, maxRetries, url,
		"确认机制测试", "请点击 {{confirm_url}} 完成确认")
}

// createConfirmReminderWithWebhookContent 同 createConfirmReminderWithWebhook，可指定标题与正文。
func createConfirmReminderWithWebhookContent(t *testing.T, db *gorm.DB, intervalSec, maxRetries int, url, title, content string) *models.Reminder {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{"method": "POST", "url": url})
	channel := &models.Channel{
		Name:   testutil.UniqueName("webhook"),
		Type:   "webhook",
		Config: cfg,
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("创建测试通道失败: %v", err)
	}
	next := time.Now().Add(time.Hour).UTC()
	reminder := &models.Reminder{
		Title:                   title,
		Content:                 content,
		Calendar:                "solar",
		ScheduleType:            "once",
		Enabled:                 true,
		RequireConfirm:          true,
		ConfirmRetryIntervalSec: intervalSec,
		ConfirmMaxRetries:       maxRetries,
		Source:                  "web",
		NextFireAt:              models.TimePtrOf(next),
	}
	if err := db.Create(reminder).Error; err != nil {
		t.Fatalf("创建测试提醒失败: %v", err)
	}
	if err := db.Create(&models.ReminderChannel{ReminderID: reminder.ID, ChannelID: channel.ID}).Error; err != nil {
		t.Fatalf("绑定测试通道失败: %v", err)
	}
	return reminder
}

// 投递失败与重试必须在服务端日志里留痕
func TestDispatchRun_LogsFailureChain(t *testing.T) {
	logs := captureLogs(t)

	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)
	dispatch.RetryDelays = []time.Duration{0, 0, 0}

	badCh := createChannel(t, db, "unknown-type")
	r := &models.Reminder{
		Title: "全失败", Calendar: "solar", ScheduleType: "once", Enabled: true, Source: "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, badCh.ID)
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	out := logs.String()
	for _, want := range []string{
		"通道类型未注册",
		"type=unknown-type",
		"投递完成",
		"status=failed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("投递失败日志缺少 %q:\n%s", want, out)
		}
	}
}

// 没有绑定通道属于用户的配置遗漏：应留下可读的 Warn
func TestDispatchRun_LogsNoChannels(t *testing.T) {
	logs := captureLogs(t)

	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	r := &models.Reminder{
		Title: "无通道", Calendar: "solar", ScheduleType: "once", Enabled: true, Source: "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	out := logs.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "没有可用的通道") {
		t.Fatalf("缺少无通道的 Warn 日志:\n%s", out)
	}
}

// 成功投递只留汇总日志
func TestDispatchRun_SuccessLoggedOnceAtInfo(t *testing.T) {
	logs := captureLogs(t)

	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)
	dispatch.RetryDelays = []time.Duration{0}

	okCh := createChannel(t, db, "mock")
	r := &models.Reminder{
		Title: "成功", Content: "正文", Calendar: "solar", ScheduleType: "once", Enabled: true, Source: "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, okCh.ID)
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	out := logs.String()
	if !strings.Contains(out, "status=success") {
		t.Fatalf("缺少成功汇总日志:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "通道发送成功") && !strings.Contains(line, "level=DEBUG") {
			t.Fatalf("单通道成功应为 DEBUG 级别:\n%s", line)
		}
	}
	if strings.Contains(out, "level=ERROR") || strings.Contains(out, "level=WARN") {
		t.Fatalf("成功投递不应产生 Warn/Error 日志:\n%s", out)
	}
}
