package scheduler

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
)

// seedPendingLog 插入一条 pending 日志，firedAt 用于模拟不同年龄。
func seedPendingLog(t *testing.T, s *Sweeper, firedAt time.Time) *models.DeliveryLog {
	t.Helper()
	dl := &models.DeliveryLog{
		ReminderID: 1,
		FiredAt:    models.TimeOf(firedAt),
		Title:      "卡住的投递",
		Content:    "正文",
		Status:     "pending",
		Source:     "web",
	}
	if err := s.db.Create(dl).Error; err != nil {
		t.Fatalf("插入 pending 日志失败: %v", err)
	}
	return dl
}

// 超过 pendingTTL 的 pending 日志应被收敛为 failed，并带上原因。
//
// 这条用例防的是「进程在投递中途退出 → 日志永远停在发送中」的回归：
// 没有收敛时日志页会一直显示「发送中」，成功率统计也被永久污染。
func TestSweeperTick_RecoversStalePendingLogs(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)
	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)

	// 20 分钟前仍是 pending：远超 pendingTTL（10 分钟）
	stale := seedPendingLog(t, s, time.Now().Add(-20*time.Minute))
	s.tick()

	var after models.DeliveryLog
	if err := db.First(&after, stale.ID).Error; err != nil {
		t.Fatalf("重读日志失败: %v", err)
	}
	if after.Status != "failed" {
		t.Fatalf("状态 = %q，期望 failed", after.Status)
	}
	if after.Error == "" {
		t.Fatal("收敛时应写清原因，否则用户无法判断为什么没发出去")
	}
}

// 还在 TTL 内、可能正在重试的日志不能被误判。
func TestSweeperTick_KeepsRecentPendingLogs(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)
	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)

	// 1 分钟前：正常投递过程，可能还在重试
	fresh := seedPendingLog(t, s, time.Now().Add(-time.Minute))
	s.tick()

	var after models.DeliveryLog
	if err := db.First(&after, fresh.ID).Error; err != nil {
		t.Fatalf("重读日志失败: %v", err)
	}
	if after.Status != "pending" {
		t.Fatalf("状态 = %q，期望仍为 pending", after.Status)
	}
}

// 已到终态的日志不应被改动。
func TestSweeperTick_DoesNotTouchFinishedLogs(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)
	s := NewSweeper(db, engine, testTZ(loc), time.Minute, 60*time.Minute, 0)

	dl := seedPendingLog(t, s, time.Now().Add(-20*time.Minute))
	if err := db.Model(&models.DeliveryLog{}).Where("id = ?", dl.ID).
		Update("status", "success").Error; err != nil {
		t.Fatalf("更新状态失败: %v", err)
	}
	s.tick()

	var after models.DeliveryLog
	if err := db.First(&after, dl.ID).Error; err != nil {
		t.Fatalf("重读日志失败: %v", err)
	}
	if after.Status != "success" {
		t.Fatalf("终态日志被改写为 %q", after.Status)
	}
}

// 默认参数下 pendingTTL 应已就位。
func TestNewSweeper_PendingTTLDefault(t *testing.T) {
	db := openSchedulerTestDB(t)
	engine := NewEngine(db, &fakeDispatcher{}, nil)
	s := NewSweeper(db, engine, nil, 0, 0, 0)
	if s.pendingTTL <= 0 {
		t.Fatalf("pendingTTL = %v，应大于 0", s.pendingTTL)
	}
}
