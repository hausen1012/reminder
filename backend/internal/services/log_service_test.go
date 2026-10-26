package services

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// testCfg 返回测试用配置。
func testCfg() *config.Config {
	return &config.Config{BaseURL: "http://base.test"}
}

// newLogSvc 构造日志服务。
func newLogSvc(t *testing.T, baseURL string) (*LogService, *gorm.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewLogService(db, baseURL), db
}

// seedDeliveryLog 插入一条投递日志。
func seedDeliveryLog(t *testing.T, db *gorm.DB, title, status string, retryRound int, chainID *string) *models.DeliveryLog {
	t.Helper()
	dl := &models.DeliveryLog{
		ReminderID:     1,
		FiredAt:        time.Now().Add(-time.Duration(retryRound) * time.Minute),
		Title:          title,
		Content:        "正文",
		Status:         status,
		Source:         "web",
		RetryRound:     retryRound,
		ConfirmChainID: chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("插入日志失败: %v", err)
	}
	return dl
}

// 详情应带出提醒标题、attempts 与确认链接。
func TestLogGetDetail(t *testing.T) {
	svc, db := newLogSvc(t, "http://base.test")

	// 提醒存在时用提醒标题
	r := &models.Reminder{Title: "提醒标题", Calendar: "solar", ScheduleType: "once", Enabled: true}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	chain := "chain-1"
	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: time.Now(), Title: "日志快照", Status: "success",
		Source: "web", ConfirmChainID: &chain,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	db.Create(&models.DeliveryAttempt{
		DeliveryLogID: dl.ID, ChannelID: 1, ChannelType: "log", ChannelName: "c", Attempt: 1, Status: "success",
	})
	if _, err := NewConfirmService(db, testCfg()).CreateToken(dl.ID, time.Hour); err != nil {
		t.Fatalf("创建确认令牌失败: %v", err)
	}

	view, err := svc.GetDetail(dl.ID)
	if err != nil {
		t.Fatalf("GetDetail 失败: %v", err)
	}
	if view.ReminderTitle != "提醒标题" {
		t.Fatalf("reminder_title = %q，期望提醒标题", view.ReminderTitle)
	}
	if len(view.Attempts) != 1 {
		t.Fatalf("attempts = %d，期望 1", len(view.Attempts))
	}
	if view.ConfirmURL == "" {
		t.Fatal("confirm_url 应有值")
	}
	if !containsSub(view.ConfirmURL, "http://base.test/c/") {
		t.Fatalf("confirm_url 拼接错误: %s", view.ConfirmURL)
	}
}

// 提醒已被删除时，详情应回退到日志快照标题。
func TestLogGetDetail_FallsBackToSnapshot(t *testing.T) {
	svc, db := newLogSvc(t, "")

	dl := seedDeliveryLog(t, db, "快照标题", "success", 0, nil)

	view, err := svc.GetDetail(dl.ID)
	if err != nil {
		t.Fatalf("GetDetail 失败: %v", err)
	}
	if view.ReminderTitle != "快照标题" {
		t.Fatalf("reminder_title = %q，期望快照标题", view.ReminderTitle)
	}
}

// 不存在的日志应返回 not_found。
func TestLogGetDetail_NotFound(t *testing.T) {
	svc, _ := newLogSvc(t, "")

	if _, err := svc.GetDetail(9999); err == nil {
		t.Fatal("不存在应报错")
	}
}

// 列表：status / source / search / since / reminder_id 过滤。
func TestLogList_Filters(t *testing.T) {
	svc, db := newLogSvc(t, "")

	ok := seedDeliveryLog(t, db, "成功的日志", "success", 0, nil)
	failed := seedDeliveryLog(t, db, "失败的日志", "failed", 0, nil)
	if err := db.Model(&models.DeliveryLog{}).Where("id = ?", failed.ID).
		Update("source", "api").Error; err != nil {
		t.Fatalf("更新 source 失败: %v", err)
	}
	_ = ok

	// status
	items, total, err := svc.List(LogFilter{Status: "failed", Limit: 10})
	if err != nil {
		t.Fatalf("过滤 status 失败: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("status=failed 应为 1 条，实际 %d", total)
	}

	// source
	_, total, err = svc.List(LogFilter{Source: "api", Limit: 10})
	if err != nil {
		t.Fatalf("过滤 source 失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("source=api 应为 1 条，实际 %d", total)
	}

	// search
	_, total, err = svc.List(LogFilter{Search: "失败", Limit: 10})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("搜索应为 1 条，实际 %d", total)
	}

	// since
	future := time.Now().Add(time.Hour)
	_, total, err = svc.List(LogFilter{Since: &future, Limit: 10})
	if err != nil {
		t.Fatalf("过滤 since 失败: %v", err)
	}
	if total != 0 {
		t.Fatalf("since=未来 应为 0 条，实际 %d", total)
	}

	// reminder_id
	_, total, err = svc.List(LogFilter{ReminderID: 999, Limit: 10})
	if err != nil {
		t.Fatalf("过滤 reminder_id 失败: %v", err)
	}
	if total != 0 {
		t.Fatalf("reminder_id=999 应为 0 条，实际 %d", total)
	}
}

// 空结果时列表应返回空切片而非 nil。
func TestLogList_Empty(t *testing.T) {
	svc, _ := newLogSvc(t, "")

	items, total, err := svc.List(LogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d，期望 0", total)
	}
	if items == nil {
		t.Fatal("应返回空切片而非 nil")
	}
}

// PurgeCount 与 Purge：按时间范围和 all 两种模式。
func TestLogPurge(t *testing.T) {
	svc, db := newLogSvc(t, "")

	// 一条旧日志、一条新日志
	old := seedDeliveryLog(t, db, "旧", "success", 0, nil)
	if err := db.Model(&models.DeliveryLog{}).Where("id = ?", old.ID).
		Update("fired_at", time.Now().Add(-48*time.Hour)).Error; err != nil {
		t.Fatalf("更新 fired_at 失败: %v", err)
	}
	seedDeliveryLog(t, db, "新", "success", 0, nil)

	// 24 小时前的只应命中旧的那条
	n, err := svc.PurgeCount(24*time.Hour, false)
	if err != nil {
		t.Fatalf("PurgeCount 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("PurgeCount = %d，期望 1", n)
	}

	deleted, err := svc.Purge(24*time.Hour, false)
	if err != nil {
		t.Fatalf("Purge 失败: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d，期望 1", deleted)
	}

	// 剩下的新日志
	_, total, _ := svc.List(LogFilter{Limit: 10})
	if total != 1 {
		t.Fatalf("应剩 1 条，实际 %d", total)
	}

	// all=true 清空全部
	if n, err := svc.PurgeCount(0, true); err != nil || n != 1 {
		t.Fatalf("PurgeCount(all) = %d, err=%v，期望 1", n, err)
	}
	if deleted, err := svc.Purge(0, true); err != nil || deleted != 1 {
		t.Fatalf("Purge(all) = %d, err=%v，期望 1", deleted, err)
	}
	if _, total, _ := svc.List(LogFilter{Limit: 10}); total != 0 {
		t.Fatalf("应已清空，实际 %d", total)
	}
}

// olderThan=0 且 all=false 时不应删除任何数据。
func TestLogPurge_Noop(t *testing.T) {
	svc, db := newLogSvc(t, "")
	seedDeliveryLog(t, db, "保留", "success", 0, nil)

	if n, err := svc.PurgeCount(0, false); err != nil || n != 0 {
		t.Fatalf("PurgeCount = %d, err=%v，期望 0", n, err)
	}
	if n, err := svc.Purge(0, false); err != nil || n != 0 {
		t.Fatalf("Purge = %d, err=%v，期望 0", n, err)
	}
	if _, total, _ := svc.List(LogFilter{Limit: 10}); total != 1 {
		t.Fatalf("数据不应被删除，实际剩 %d", total)
	}
}

// BatchDelete 应连带清理 attempts 与孤立 confirm_tokens。
func TestLogBatchDelete(t *testing.T) {
	svc, db := newLogSvc(t, "")

	dl := seedDeliveryLog(t, db, "待删", "success", 0, nil)
	db.Create(&models.DeliveryAttempt{
		DeliveryLogID: dl.ID, ChannelID: 1, ChannelType: "log", ChannelName: "c", Attempt: 1, Status: "success",
	})
	if _, err := NewConfirmService(db, testCfg()).CreateToken(dl.ID, time.Hour); err != nil {
		t.Fatalf("创建确认令牌失败: %v", err)
	}

	if err := svc.BatchDelete([]uint{dl.ID}); err != nil {
		t.Fatalf("BatchDelete 失败: %v", err)
	}

	var attempts, tokens int64
	db.Model(&models.DeliveryAttempt{}).Where("delivery_log_id = ?", dl.ID).Count(&attempts)
	db.Model(&models.ConfirmToken{}).Where("delivery_log_id = ?", dl.ID).Count(&tokens)
	if attempts != 0 {
		t.Fatalf("attempts 未清理，剩 %d", attempts)
	}
	if tokens != 0 {
		t.Fatalf("孤立 confirm_tokens 未清理，剩 %d", tokens)
	}

	// 空 ids 不应报错
	if err := svc.BatchDelete(nil); err != nil {
		t.Fatalf("空 ids 不应报错: %v", err)
	}
}

// NewLogService 不传 baseURL 时应使用默认值。
func TestNewLogService_DefaultURL(t *testing.T) {
	db := openTestDB(t)

	svc := NewLogService(db)
	if svc.PublicURL == "" {
		t.Fatal("应使用默认 PublicURL")
	}

	svc2 := NewLogService(db, "")
	if svc2.PublicURL == "" {
		t.Fatal("空字符串应回退到默认 PublicURL")
	}
}
