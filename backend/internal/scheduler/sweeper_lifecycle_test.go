package scheduler

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
)

// Sweeper 生命周期：Start → IsRunning → Interval → Stop。
func TestSweeperStartStop(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	s := NewSweeper(db, engine, testTZ(loc), 100*time.Millisecond, time.Hour, 30*time.Second)

	if s.IsRunning() {
		t.Fatal("未 Start 前不应 running")
	}

	s.Start()
	if !s.IsRunning() {
		t.Fatal("Start 后应为 running")
	}
	if got := s.Interval(); got != 100*time.Millisecond {
		t.Fatalf("Interval = %v", got)
	}

	// 重复 Start 应幂等
	s.Start()
	if !s.IsRunning() {
		t.Fatal("重复 Start 后仍应 running")
	}

	s.Stop()
	if s.IsRunning() {
		t.Fatal("Stop 后不应 running")
	}

	// 重复 Stop 不应 panic
	s.Stop()
}

// NewSweeper 对非法参数应使用默认值。
func TestNewSweeper_Defaults(t *testing.T) {
	db := openSchedulerTestDB(t)
	engine := NewEngine(db, &fakeDispatcher{}, nil)

	s := NewSweeper(db, engine, nil, 0, 0, 0)
	if s.Interval() != 60*time.Second {
		t.Fatalf("interval 默认值 = %v，期望 60s", s.Interval())
	}
	if s.missTol != 60*time.Minute {
		t.Fatalf("missTol 默认值 = %v，期望 60m", s.missTol)
	}
	if s.skewSeconds != 30*time.Second {
		t.Fatalf("skew 默认值 = %v，期望 30s", s.skewSeconds)
	}
}

// loop 应按 interval 周期执行 tick（用短 interval 观察）。
func TestSweeperLoop_RunsPeriodically(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	// 插入一条严重过期的提醒，sweeper 第一次 tick 就会处理它
	missed := time.Now().Add(-48 * time.Hour).In(loc)
	r := &models.Reminder{
		Title:        "会被 sweep",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: datatypes.JSON(`{"at":"2020-01-01T09:00:00"}`),
		Enabled:      true,
		Source:       "web",
		NextFireAt:   &missed,
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	s := NewSweeper(db, engine, testTZ(loc), 20*time.Millisecond, time.Hour, 0)
	s.Start()
	defer s.Stop()

	// 等待第一次 tick 生效
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var after models.Reminder
		if err := db.First(&after, r.ID).Error; err == nil && !after.Enabled {
			return // 成功：已被标记过期并禁用
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("sweeper loop 未在预期时间内处理过期提醒")
}
