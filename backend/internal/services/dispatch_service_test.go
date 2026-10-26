package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// createChannel 创建一个测试用通道。
func createChannel(t *testing.T, db *gorm.DB, chType string, enabled bool) *models.Channel {
	t.Helper()
	ch := &models.Channel{
		Name:    fmt.Sprintf("%s-%d", chType, time.Now().UnixNano()),
		Type:    chType,
		Enabled: enabled,
		Config:  []byte("{}"),
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
		FiredAt:    time.Now(),
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
	dispatch := NewDispatchService(db, NewChannelService(db, box), loc)
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

	ch1 := createChannel(t, db, "log", true)
	ch2 := createChannel(t, db, "log", true)

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

	okCh := createChannel(t, db, "log", true)
	// 不支持的通道类型会在 Get(type) 阶段失败，模拟一个必定失败的通道
	badCh := createChannel(t, db, "unknown-type", true)

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

	badCh := createChannel(t, db, "unknown-type", true)

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

// 禁用的通道不应被加载（等同没有可用通道）。
func TestDispatchRun_DisabledChannelSkipped(t *testing.T) {
	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	disabled := createChannel(t, db, "log", false)

	r := &models.Reminder{
		Title:        "禁用通道",
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, disabled.ID)
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	var got models.DeliveryLog
	if err := db.First(&got, logID).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("禁用通道应视为无可用通道 → failed，实际 %q", got.Status)
	}
}

// 空参数应直接返回，不 panic。
func TestDispatchRun_NilOrZeroArgs(t *testing.T) {
	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	dispatch.Run(context.Background(), nil, 1)
	dispatch.Run(context.Background(), &models.Reminder{}, 0)
}
