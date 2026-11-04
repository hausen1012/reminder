package services

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// chainSeed 描述一条待造的确认链。
type chainSeed struct {
	chainID string
	// rounds 是这条链已发出的轮次（从 0 开始，值越大越新）
	rounds []int
	// lastFiredAgo 是最后一轮距当前多久
	lastFiredAgo time.Duration
	// tokenTTL 是令牌有效期；0 表示不建令牌
	tokenTTL time.Duration
	// confirmedRound 非 nil 时把该轮次标记为已确认。
	// 用指针而不是 int：零值 0 会被误当成「标记第 0 轮」，
	// 那样每条用例的第一轮都会变成已确认的链。
	confirmedRound *int
}

func intPtr(v int) *int { return &v }

// seedChain 按描述造一条完整的确认链：若干轮日志 + 挂在首条日志上的令牌。
//
// 令牌必须挂在首条日志上，与 Dispatch 的行为一致（CreateToken 传的是首次触发的
// 日志 ID）；后续重发轮没有对应的 token 行。
func seedChain(t *testing.T, db *gorm.DB, reminderID uint, s chainSeed) {
	t.Helper()
	var firstLogID uint
	for i, round := range s.rounds {
		firedAgo := s.lastFiredAgo + time.Duration(len(s.rounds)-1-i)*time.Minute
		row := &models.DeliveryLog{
			ReminderID:     reminderID,
			FiredAt:        models.TimeOf(time.Now().Add(-firedAgo)),
			Title:          "t",
			Content:        "正文",
			Status:         "success",
			Source:         "web",
			RetryRound:     round,
			ConfirmChainID: &s.chainID,
			Confirmed:      s.confirmedRound != nil && round == *s.confirmedRound,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("创建链日志失败: %v", err)
		}
		if i == 0 {
			firstLogID = row.ID
		}
	}
	if s.tokenTTL != 0 {
		token := &models.ConfirmToken{
			Token:         "tok-" + s.chainID,
			DeliveryLogID: firstLogID,
			ExpiresAt:     models.TimeOf(time.Now().Add(s.tokenTTL)),
		}
		if err := db.Create(token).Error; err != nil {
			t.Fatalf("创建令牌失败: %v", err)
		}
	}
}

// 进程重启后，未完成的确认链应按库里的轮次继续排下一轮。
//
// 这是「重启导致催办丢失」的复现用例：不恢复时定时器随进程消失，
// 一个需要确认的提醒只会发一次，永远不会再催。
func TestRecoverChains_ResumesUnconfirmedChain(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-restart-1",
		rounds:       []int{0, 1, 2},
		lastFiredAgo: 10 * time.Minute,
		tokenTTL:     time.Hour,
	})

	n, err := stack.confirmMgr.RecoverChains()
	if err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("恢复数量 = %d，期望 1", n)
	}
	if !stack.hasTimer("chain-restart-1") {
		t.Fatal("未完成链应被重新挂上定时器")
	}
}

// 确认只标在较早的某一轮、最新一轮仍是未确认时，整条链仍应视为已完成。
//
// 这是「确认与重发竞态」的回归用例：用户点了确认之后，仍可能有一条
// confirmed=false 的重发日志落库。只看最新一条的 confirmed 会把它当成活的，
// 重启后再催一遍已经处理过的事。
func TestRecoverChains_SkipsChainConfirmedOnEarlierRound(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)
	seedChain(t, db, r.ID, chainSeed{
		chainID:        "chain-race",
		rounds:         []int{0, 1, 2},
		lastFiredAgo:   5 * time.Minute,
		tokenTTL:       time.Hour,
		confirmedRound: intPtr(1),
	})

	n, err := stack.confirmMgr.RecoverChains()
	if err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if n != 0 || stack.hasTimer("chain-race") {
		t.Fatal("链里有任何一轮已确认时，整条链都不应恢复")
	}
}

// 已确认的链不应恢复。
func TestRecoverChains_SkipsConfirmedChain(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)
	seedChain(t, db, r.ID, chainSeed{
		chainID:        "chain-confirmed-restart",
		rounds:         []int{0},
		lastFiredAgo:   time.Minute,
		tokenTTL:       time.Hour,
		confirmedRound: intPtr(0),
	})

	n, err := stack.confirmMgr.RecoverChains()
	if err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if n != 0 || stack.hasTimer("chain-confirmed-restart") {
		t.Fatal("已确认的链不应恢复")
	}
}

// 同一个提醒留下多条历史链时，只恢复最新那条。
//
// 旧链的链接用户已经拿不到了（消息早被新消息顶掉），恢复它们只会在重启后
// 变成多条催办链并发往手机上发。
func TestRecoverChains_OnlyNewestChainPerReminder(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)

	// 旧链：两小时前开始，轮次 0..1
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-old",
		rounds:       []int{0, 1},
		lastFiredAgo: 2 * time.Hour,
		tokenTTL:     48 * time.Hour,
	})
	// 新链：十分钟前开始，轮次 0
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-new",
		rounds:       []int{0},
		lastFiredAgo: 10 * time.Minute,
		tokenTTL:     time.Hour,
	})

	n, err := stack.confirmMgr.RecoverChains()
	if err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("恢复数量 = %d，期望 1（每个提醒只恢复最新链）", n)
	}
	if stack.hasTimer("chain-old") {
		t.Fatal("历史链不应恢复")
	}
	if !stack.hasTimer("chain-new") {
		t.Fatal("最新链应被恢复")
	}
}

// 已经用完最大重发次数的链不应恢复。
func TestRecoverChains_SkipsExhaustedChain(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 2) // 最多重发 2 次
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-exhausted",
		rounds:       []int{0, 1, 2},
		lastFiredAgo: time.Minute,
		tokenTTL:     time.Hour,
	})

	if _, err := stack.confirmMgr.RecoverChains(); err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if stack.hasTimer("chain-exhausted") {
		t.Fatal("轮次已用完的链不应恢复")
	}
}

// 确认链接已过期的链不应恢复：链接点不开，继续催只是骚扰。
func TestRecoverChains_SkipsExpiredToken(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-token-expired",
		rounds:       []int{0, 1},
		lastFiredAgo: 5 * time.Minute,
		tokenTTL:     -time.Hour,
	})

	n, err := stack.confirmMgr.RecoverChains()
	if err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if n != 0 || stack.hasTimer("chain-token-expired") {
		t.Fatal("令牌已过期的链不应恢复")
	}
}

// 没有令牌的链（例如创建令牌失败）不应恢复。
func TestRecoverChains_SkipsChainWithoutToken(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-no-token",
		rounds:       []int{0},
		lastFiredAgo: time.Minute,
	})

	if _, err := stack.confirmMgr.RecoverChains(); err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if stack.hasTimer("chain-no-token") {
		t.Fatal("没有令牌的链不应恢复")
	}
}

// 提醒已禁用或已关闭确认时不应恢复。
func TestRecoverChains_SkipsInactiveReminder(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-disabled",
		rounds:       []int{0},
		lastFiredAgo: time.Minute,
		tokenTTL:     time.Hour,
	})
	if err := db.Model(&models.Reminder{}).Where("id = ?", r.ID).
		Update("enabled", false).Error; err != nil {
		t.Fatalf("禁用提醒失败: %v", err)
	}

	if _, err := stack.confirmMgr.RecoverChains(); err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if stack.hasTimer("chain-disabled") {
		t.Fatal("已禁用提醒的链不应恢复")
	}
}

// 停机时间超过重发间隔时，应立即补一轮，而不是再等一个完整间隔。
func TestRecoverChains_FiresImmediatelyWhenIntervalAlreadyPassed(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5) // 间隔 1 秒
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-overdue",
		rounds:       []int{0},
		lastFiredAgo: 5 * time.Minute,
		tokenTTL:     time.Hour,
	})

	before := countLogsByReminder(t, db, r.ID)
	if _, err := stack.confirmMgr.RecoverChains(); err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		return countLogsByReminder(t, db, r.ID) > before
	}, "停机超过间隔后应立即重发一轮")
}

// 停机时间不到一个间隔时，应只等剩余时间；刚重启不应立刻发。
func TestRecoverChains_WaitsRemainingInterval(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 5, 5) // 间隔 5 秒
	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-fresh",
		rounds:       []int{0},
		lastFiredAgo: 0,
		tokenTTL:     time.Hour,
	})

	before := countLogsByReminder(t, db, r.ID)
	if _, err := stack.confirmMgr.RecoverChains(); err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	if got := countLogsByReminder(t, db, r.ID); got != before {
		t.Fatalf("剩余间隔未到就重发了：日志 %d -> %d", before, got)
	}
}

// 没有未完成链时应安静返回 0。
func TestRecoverChains_NoChains(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	n, err := stack.confirmMgr.RecoverChains()
	if err != nil {
		t.Fatalf("RecoverChains 失败: %v", err)
	}
	if n != 0 {
		t.Fatalf("恢复数量 = %d，期望 0", n)
	}
}

// 恢复阶段的查询数不应随链数量增长（防 N+1）。
//
// 用带计数 logger 的 gorm session 跑恢复：先恢复 1 条链，再恢复 21 条链，
// 两次的查询数应当相同——链多时只是同一批查询的结果更多，不增加往返次数。
func TestRecoverChains_QueryCountIndependentOfChainCount(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	// 用独立 manager 跑恢复，才能让它落在带计数 logger 的 session 上
	recoverWith := func(scoped *gorm.DB) int {
		mgr := NewConfirmRetryManager(scoped, stack.dispatch, stack.confirmSvc, testTZ(cstZone()))
		n, err := mgr.RecoverChains()
		if err != nil {
			t.Fatalf("RecoverChains 失败: %v", err)
		}
		mgr.StopAll()
		return n
	}

	// 第一个提醒：1 条链
	r1 := createConfirmReminder(t, db, 60, 5)
	seedChain(t, db, r1.ID, chainSeed{
		chainID: "chain-q1", rounds: []int{0}, lastFiredAgo: 0, tokenTTL: time.Hour,
	})
	queriesOne := countQueries(t, db, func(scoped *gorm.DB) { recoverWith(scoped) })

	// 再加 20 个提醒，每个 1 条链
	for i := 0; i < 20; i++ {
		ri := createConfirmReminder(t, db, 60, 5)
		seedChain(t, db, ri.ID, chainSeed{
			chainID:      "chain-q-" + itoaTest(i),
			rounds:       []int{0},
			lastFiredAgo: 0, // 刚发过：恢复时只排定时器，不会在计数期间触发 retry
			tokenTTL:     time.Hour,
		})
	}
	queriesMany := countQueries(t, db, func(scoped *gorm.DB) {
		if got := recoverWith(scoped); got != 21 {
			t.Fatalf("恢复数量 = %d，期望 21", got)
		}
	})

	if queriesMany > queriesOne {
		t.Fatalf("查询数随链数量增长：1 条链 %d 次，21 条链 %d 次（存在 N+1）", queriesOne, queriesMany)
	}
}
