// 本包测试脚手架集中处。
//
// 内存建库、固定时区、fake 替身与确认链造数辅助原本散落在各用例文件里
// （尤其是 confirm_flow_test.go 被其余二十来个文件依赖），现在统一收到这里。
package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/testutil"
	"github.com/reminder/backend/internal/timezone"
	"gorm.io/gorm"
)

// openTestDB 构造测试用的内存 SQLite 库（已迁移生产全量模型）。
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewDB(t)
}

// testTZ 把固定时区包装成 timezone.Provider，供测试注入。
func testTZ(loc *time.Location) *timezone.Provider {
	return testutil.TZ(loc)
}

// cstZone 是测试默认时区（东八区），与生产默认值一致。
func cstZone() *time.Location {
	return testutil.CST()
}

// testCfg 返回测试用配置。
func testCfg() *config.Config {
	return &config.Config{BaseURL: "http://example.test"}
}

// containsSub 判断字符串是否包含子串。
func containsSub(s, sub string) bool {
	return strings.Contains(s, sub)
}

// noopEngine 是 SchedulerHook 的空实现，避免测试启动真实调度。
type noopEngine struct{}

func (noopEngine) Add(*models.Reminder) error    { return nil }
func (noopEngine) Update(*models.Reminder) error { return nil }
func (noopEngine) Remove(uint)                   {}

// newChannelSvcForTest 构造通道服务。
func newChannelSvcForTest(t *testing.T) (*ChannelService, *gorm.DB) {
	t.Helper()
	db := openTestDB(t)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	return NewChannelService(db, box, testTZ(cstZone())), db
}

// confirmTestStack 汇集确认重发链路涉及的各服务，便于端到端断言。
type confirmTestStack struct {
	dispatch    *DispatchService
	confirmSvc  *ConfirmService
	confirmMgr  *ConfirmRetryManager
	reminderSvc *ReminderService
}

// newConfirmTestStack 用真实服务拼出确认链路（投递不重试，加快用例）。
func newConfirmTestStack(t *testing.T, db *gorm.DB) *confirmTestStack {
	t.Helper()
	tz := testTZ(cstZone())
	channelSvc := &ChannelService{DB: db, TZ: tz}
	dispatch := NewDispatchService(db, channelSvc, tz)
	dispatch.RetryDelays = []time.Duration{0}
	confirmSvc := NewConfirmService(db, &config.Config{BaseURL: "http://example.test"})
	confirmMgr := NewConfirmRetryManager(db, dispatch, confirmSvc, tz)
	dispatch.ConfirmMgr = confirmMgr
	reminderSvc := NewReminderService(db, noopEngine{}, tz, dispatch, confirmMgr)
	stack := &confirmTestStack{
		dispatch:    dispatch,
		confirmSvc:  confirmSvc,
		confirmMgr:  confirmMgr,
		reminderSvc: reminderSvc,
	}
	t.Cleanup(confirmMgr.StopAll)
	return stack
}

// hasTimer 判断某条确认链当前是否挂在定时器上。
func (s *confirmTestStack) hasTimer(chainID string) bool {
	s.confirmMgr.mu.Lock()
	defer s.confirmMgr.mu.Unlock()
	_, ok := s.confirmMgr.timers[chainID]
	return ok
}

// triggerDispatch 创建 pending 日志后调 Run，模拟一次触发。
func triggerDispatch(ctx context.Context, d *DispatchService, r *models.Reminder) (uint, error) {
	now := time.Now()
	dlog := &models.DeliveryLog{
		ReminderID: r.ID,
		FiredAt:    models.TimeOf(now),
		Title:      r.Title,
		Content:    r.Content,
		Status:     "pending",
		Source:     r.Source,
		RetryRound: 0,
	}
	if err := d.DB.Create(dlog).Error; err != nil {
		return 0, err
	}
	d.Run(ctx, r, dlog.ID)
	return dlog.ID, nil
}

// createConfirmReminder 造一条需要确认、绑定了 log 通道的提醒。
func createConfirmReminder(t *testing.T, db *gorm.DB, intervalSec, maxRetries int) *models.Reminder {
	t.Helper()
	channel := &models.Channel{
		Name:    testutil.UniqueName("log"),
		Type:    "log",
		Enabled: true,
		Config:  []byte("{}"),
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("创建测试通道失败: %v", err)
	}
	next := time.Now().Add(time.Hour).UTC()
	reminder := &models.Reminder{
		Title:                   "确认机制测试",
		Content:                 "请点击 {{confirm_url}} 完成确认",
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

// countLogsByReminder 统计某提醒的投递日志条数。
func countLogsByReminder(t *testing.T, db *gorm.DB, reminderID uint) int {
	t.Helper()
	var count int64
	if err := db.Model(&models.DeliveryLog{}).Where("reminder_id = ?", reminderID).Count(&count).Error; err != nil {
		t.Fatalf("统计日志失败: %v", err)
	}
	return int(count)
}

// listLogsByReminder 按重发轮次升序列出某提醒的日志。
func listLogsByReminder(t *testing.T, db *gorm.DB, reminderID uint) []models.DeliveryLog {
	t.Helper()
	var logs []models.DeliveryLog
	if err := db.Where("reminder_id = ?", reminderID).Order("retry_round ASC, id ASC").Find(&logs).Error; err != nil {
		t.Fatalf("查询提醒日志失败: %v", err)
	}
	return logs
}

// listLogsByChain 按重发轮次升序列出某条确认链的日志。
func listLogsByChain(t *testing.T, db *gorm.DB, chainID string) []models.DeliveryLog {
	t.Helper()
	var logs []models.DeliveryLog
	if err := db.Where("confirm_chain_id = ?", chainID).Order("retry_round ASC, id ASC").Find(&logs).Error; err != nil {
		t.Fatalf("查询确认链日志失败: %v", err)
	}
	return logs
}

// getLogByID 按 id 取一条日志。
func getLogByID(t *testing.T, db *gorm.DB, id uint) models.DeliveryLog {
	t.Helper()
	var logRow models.DeliveryLog
	if err := db.First(&logRow, id).Error; err != nil {
		t.Fatalf("查询日志失败: %v", err)
	}
	return logRow
}

// getTokenByChain 取某条确认链上的确认令牌。
func getTokenByChain(t *testing.T, db *gorm.DB, chainID string) models.ConfirmToken {
	t.Helper()
	var tok models.ConfirmToken
	err := db.Joins("JOIN delivery_logs ON delivery_logs.id = confirm_tokens.delivery_log_id").
		Where("delivery_logs.confirm_chain_id = ?", chainID).
		First(&tok).Error
	if err != nil {
		t.Fatalf("查询 token 失败: %v", err)
	}
	return tok
}

// waitFor 轮询等待条件成立，超时即测试失败。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("超时：%s", desc)
}
