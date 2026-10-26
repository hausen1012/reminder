package services

import (
	"context"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// newDispatchForDryRun 构造仅用于试发的 dispatch 服务。
func newDispatchForDryRun(t *testing.T, db *gorm.DB) *DispatchService {
	t.Helper()
	loc := time.FixedZone("CST", 8*3600)
	d := NewDispatchService(db, &ChannelService{DB: db}, loc)
	d.RetryDelays = []time.Duration{0}
	return d
}

// DryRun 全成功时不返回错误，且不落任何日志/attempt。
func TestDispatchDryRun_AllSucceedNoRecords(t *testing.T) {
	db := openTestDB(t)
	d := newDispatchForDryRun(t, db)

	ch1 := createChannel(t, db, "log", true)
	ch2 := createChannel(t, db, "log", true)

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

	bad := &models.Channel{Name: "bad", Type: "unknown", Enabled: true, Config: []byte("{}")}
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
	bad := &models.Channel{Name: "x", Type: "unknown", Enabled: true, Config: []byte("{}")}

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
		"trigger_at", "trigger_date", "trigger_time", "weekday",
		"title", "content", "lunar_date", "reminder_id", "reminder_source", "source",
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
	// lunar_date 应含"农历"
	if !containsSub(vars["lunar_date"], "农历") {
		t.Fatalf("lunar_date = %q", vars["lunar_date"])
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
