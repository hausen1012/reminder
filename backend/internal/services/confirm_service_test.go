package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// newConfirmSvc 构造确认服务。
func newConfirmSvc(t *testing.T, siteURL string) (*ConfirmService, *gorm.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewConfirmService(db, &config.Config{SiteURL: siteURL}), db
}

// seedLogWithChain 插入一条可用于确认的投递日志。
func seedLogWithChain(t *testing.T, db *gorm.DB, chainID *string) *models.DeliveryLog {
	t.Helper()
	dl := &models.DeliveryLog{
		ReminderID:     1,
		FiredAt:        models.TimeNow(),
		Title:          "待确认",
		Content:        "正文",
		Status:         "success",
		Source:         "web",
		ConfirmChainID: chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	return dl
}

// 令牌应是 64 位 hex，且落库。
func TestConfirmCreateToken(t *testing.T) {
	svc, db := newConfirmSvc(t, "http://x.test")

	dl := seedLogWithChain(t, db, nil)
	token, err := svc.CreateToken(dl.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("令牌长度 = %d，期望 64（32 字节 hex）", len(token))
	}

	var ct models.ConfirmToken
	if err := db.Where("token = ?", token).First(&ct).Error; err != nil {
		t.Fatalf("令牌未落库: %v", err)
	}
	if ct.DeliveryLogID != dl.ID {
		t.Fatalf("delivery_log_id = %d，期望 %d", ct.DeliveryLogID, dl.ID)
	}
	if ct.UsedAt != nil {
		t.Fatal("新令牌 used_at 应为 nil")
	}
}

// 消费令牌：标记 used_at，并把该链上所有日志置为 confirmed。
func TestConfirmConsumeToken_WithChain(t *testing.T) {
	svc, db := newConfirmSvc(t, "http://x.test")

	chain := "chain-abc"
	first := seedLogWithChain(t, db, &chain)
	// 同一链上的重发记录
	retry := &models.DeliveryLog{
		ReminderID: 1, FiredAt: models.TimeNow(), Title: "重发", Status: "success",
		Source: "web", RetryRound: 1, ConfirmChainID: &chain,
	}
	if err := db.Create(retry).Error; err != nil {
		t.Fatalf("创建重发日志失败: %v", err)
	}

	token, err := svc.CreateToken(first.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	dl, err := svc.ConsumeToken(token)
	if err != nil {
		t.Fatalf("消费令牌失败: %v", err)
	}
	if dl.Confirmed != true && !dl.Confirmed {
		t.Fatalf("返回的日志应已确认: %+v", dl)
	}

	// 链上两条都应 confirmed
	var rows []models.DeliveryLog
	db.Where("confirm_chain_id = ?", chain).Find(&rows)
	if len(rows) != 2 {
		t.Fatalf("应查到 2 条链上日志，实际 %d", len(rows))
	}
	for _, r := range rows {
		if !r.Confirmed {
			t.Fatalf("日志 %d 未标记 confirmed", r.ID)
		}
		if r.ConfirmedAt == nil {
			t.Fatalf("日志 %d 缺少 confirmed_at", r.ID)
		}
	}
}

// 无 chain_id 时只确认单条。
func TestConfirmConsumeToken_WithoutChain(t *testing.T) {
	svc, db := newConfirmSvc(t, "http://x.test")

	dl := seedLogWithChain(t, db, nil)
	token, err := svc.CreateToken(dl.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	if _, err := svc.ConsumeToken(token); err != nil {
		t.Fatalf("消费失败: %v", err)
	}

	var got models.DeliveryLog
	if err := db.First(&got, dl.ID).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !got.Confirmed {
		t.Fatal("单条日志应被确认")
	}
	if got.ConfirmedAt == nil {
		t.Fatal("应写入 confirmed_at")
	}
}

// 令牌不存在 / 已使用 / 已过期 都应报错。
func TestConfirmConsumeToken_Errors(t *testing.T) {
	svc, db := newConfirmSvc(t, "http://x.test")

	// 不存在
	if _, err := svc.ConsumeToken("nonexistent"); err == nil {
		t.Fatal("不存在的令牌应报错")
	}

	// 已使用
	dl := seedLogWithChain(t, db, nil)
	token, err := svc.CreateToken(dl.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}
	if _, err := svc.ConsumeToken(token); err != nil {
		t.Fatalf("首次消费应成功: %v", err)
	}
	if _, err := svc.ConsumeToken(token); err == nil {
		t.Fatal("重复消费应报错")
	}

	// 已过期
	dl2 := seedLogWithChain(t, db, nil)
	expired, err := svc.CreateToken(dl2.ID, -time.Minute)
	if err != nil {
		t.Fatalf("创建过期令牌失败: %v", err)
	}
	if _, err := svc.ConsumeToken(expired); err == nil {
		t.Fatal("过期令牌应报错")
	}
}

// 令牌存在但对应日志已被删除时应报错。
func TestConfirmConsumeToken_LogDeleted(t *testing.T) {
	svc, db := newConfirmSvc(t, "http://x.test")

	dl := seedLogWithChain(t, db, nil)
	token, err := svc.CreateToken(dl.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	// 物理删除日志，令牌仍在
	if err := db.Unscoped().Delete(&models.DeliveryLog{}, dl.ID).Error; err != nil {
		t.Fatalf("删除日志失败: %v", err)
	}

	if _, err := svc.ConsumeToken(token); !errors.Is(err, ErrConfirmLogMissing) {
		t.Fatalf("日志不存在时应返回 ErrConfirmLogMissing，实际 %v", err)
	}
}

// 重复消费：返回 ErrConfirmTokenUsed 与日志本身，且不改写 confirmed_at。
func TestConfirmConsumeToken_DuplicateReturnsLog(t *testing.T) {
	svc, db := newConfirmSvc(t, "http://x.test")

	dl := seedLogWithChain(t, db, nil)
	token, err := svc.CreateToken(dl.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	first, err := svc.ConsumeToken(token)
	if err != nil {
		t.Fatalf("首次消费应成功: %v", err)
	}
	if first.ConfirmedAt == nil {
		t.Fatal("首次消费应写入 confirmed_at")
	}
	firstAt := first.ConfirmedAt.Time

	second, err := svc.ConsumeToken(token)
	if !errors.Is(err, ErrConfirmTokenUsed) {
		t.Fatalf("重复消费应返回 ErrConfirmTokenUsed，实际 %v", err)
	}
	if second == nil || second.ID != dl.ID {
		t.Fatalf("重复消费应带回日志，实际 %+v", second)
	}

	// 首次确认时间不能被第二次点击覆盖
	var got models.DeliveryLog
	if err := db.First(&got, dl.ID).Error; err != nil {
		t.Fatalf("读取日志失败: %v", err)
	}
	if got.ConfirmedAt == nil || !got.ConfirmedAt.Time.Equal(firstAt) {
		t.Fatalf("confirmed_at 被改写: %v -> %v", firstAt, got.ConfirmedAt)
	}
}

// 并发点击同一个链接：只有一个能真正消费，其余都应按「已使用」返回。
func TestConfirmConsumeToken_ConcurrentOnlyOneWins(t *testing.T) {
	svc, db := newConfirmSvc(t, "http://x.test")

	dl := seedLogWithChain(t, db, nil)
	token, err := svc.CreateToken(dl.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	const workers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	success, used, other := 0, 0, 0
	var firstErr error
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.ConsumeToken(token)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case errors.Is(err, ErrConfirmTokenUsed):
				used++
			default:
				other++
				if firstErr == nil {
					firstErr = err
				}
			}
		}()
	}
	wg.Wait()

	if success != 1 {
		t.Fatalf("并发点击应只有 1 次成功，实际 %d（other=%d, err=%v）", success, other, firstErr)
	}
	if used != workers-1 {
		t.Fatalf("其余 %d 次应返回已使用，实际 %d", workers-1, used)
	}

	var ct models.ConfirmToken
	if err := db.Where("token = ?", token).First(&ct).Error; err != nil {
		t.Fatalf("读取令牌失败: %v", err)
	}
	if ct.UsedAt == nil {
		t.Fatal("used_at 应已写入")
	}
}

// BuildURL 应拼接 base/c/token，base 为空时用默认值。
func TestConfirmBuildURL(t *testing.T) {
	db := openTestDB(t)

	svc := NewConfirmService(db, &config.Config{SiteURL: "https://example.com"})
	if got := svc.BuildURL("abc"); got != "https://example.com/c/abc" {
		t.Fatalf("BuildURL = %q", got)
	}

	empty := NewConfirmService(db, &config.Config{SiteURL: ""})
	if got := empty.BuildURL("abc"); got == "" || !containsSub(got, "/c/abc") {
		t.Fatalf("base 为空时应使用默认值，实际 %q", got)
	}
}

// 库里配置的站点地址优先于环境变量，且改完立刻对之后的拼接生效（无需重启）。
func TestConfirmBuildURL_DBOverridesEnv(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfirmService(db, &config.Config{SiteURL: "https://from-env.example"})

	if got := svc.BuildURL("t1"); got != "https://from-env.example/c/t1" {
		t.Fatalf("未落库时应回退环境变量，实际 %q", got)
	}

	if err := db.Exec("INSERT INTO configs (key, value) VALUES (?, ?)", KeySiteURL, "https://from-db.example/").Error; err != nil {
		t.Fatalf("写库失败: %v", err)
	}
	// 同一个服务实例，不重建：下一次拼接就走库里的值
	if got := svc.BuildURL("t2"); got != "https://from-db.example/c/t2" {
		t.Fatalf("应以库里的地址为准并去掉结尾斜杠，实际 %q", got)
	}
}

// InspectToken 应只查状态不消费令牌，并准确反映已使用/过期/不存在等状态。
func TestConfirmInspectToken(t *testing.T) {
	svc, db := newConfirmSvc(t, "http://x.test")

	// 1. 正常未使用
	dl := seedLogWithChain(t, db, nil)
	token, err := svc.CreateToken(dl.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	got, err := svc.InspectToken(token)
	if err != nil {
		t.Fatalf("InspectToken 失败: %v", err)
	}
	if got.ID != dl.ID {
		t.Fatalf("返回日志 ID = %d, 期望 %d", got.ID, dl.ID)
	}
	if got.Confirmed {
		t.Fatal("InspectToken 不应标记 confirmed")
	}

	var ct models.ConfirmToken
	if err := db.Where("token = ?", token).First(&ct).Error; err != nil {
		t.Fatalf("查令牌失败: %v", err)
	}
	if ct.UsedAt != nil {
		t.Fatal("InspectToken 不应写入 used_at")
	}

	// 2. 已消费后查询
	if _, err := svc.ConsumeToken(token); err != nil {
		t.Fatalf("消费令牌失败: %v", err)
	}
	gotUsed, err := svc.InspectToken(token)
	if !errors.Is(err, ErrConfirmTokenUsed) {
		t.Fatalf("已使用令牌应返回 ErrConfirmTokenUsed，实际 %v", err)
	}
	if gotUsed == nil || gotUsed.ID != dl.ID {
		t.Fatalf("已使用令牌仍应带回日志对象，实际 %+v", gotUsed)
	}

	// 3. 已过期
	dlExpired := seedLogWithChain(t, db, nil)
	expiredToken, err := svc.CreateToken(dlExpired.ID, -time.Hour)
	if err != nil {
		t.Fatalf("创建过期令牌失败: %v", err)
	}
	if _, err := svc.InspectToken(expiredToken); !errors.Is(err, ErrConfirmTokenExpired) {
		t.Fatalf("过期令牌应返回 ErrConfirmTokenExpired，实际 %v", err)
	}

	// 4. 不存在
	if _, err := svc.InspectToken("nonexistent-token"); !errors.Is(err, ErrConfirmTokenInvalid) {
		t.Fatalf("不存在令牌应返回 ErrConfirmTokenInvalid，实际 %v", err)
	}

	// 5. 对应日志已删除
	dlDeleted := seedLogWithChain(t, db, nil)
	delToken, err := svc.CreateToken(dlDeleted.ID, time.Hour)
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}
	db.Unscoped().Delete(&models.DeliveryLog{}, dlDeleted.ID)
	if _, err := svc.InspectToken(delToken); !errors.Is(err, ErrConfirmLogMissing) {
		t.Fatalf("日志已删除应返回 ErrConfirmLogMissing，实际 %v", err)
	}
}

func TestConfirmRetryStopsAtMaxRetries(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)
	reminder := createConfirmReminder(t, db, 1, 10)

	logID, err := triggerDispatch(context.Background(), stack.dispatch, reminder)
	if err != nil {
		t.Fatalf("triggerDispatch 失败: %v", err)
	}

	waitFor(t, 15*time.Second, func() bool {
		return countLogsByReminder(t, db, reminder.ID) == 11
	}, "等待重试达到 10 次")

	logs := listLogsByReminder(t, db, reminder.ID)
	if len(logs) != 11 {
		t.Fatalf("期望 11 条日志，实际 %d", len(logs))
	}
	if logs[0].ID != logID {
		t.Fatalf("首条日志 id 不匹配，期望 %d，实际 %d", logID, logs[0].ID)
	}
	if logs[0].ConfirmChainID == nil || *logs[0].ConfirmChainID == "" {
		t.Fatal("首条日志缺少 confirm_chain_id")
	}
	chainID := *logs[0].ConfirmChainID
	for i, row := range logs {
		if row.RetryRound != i {
			t.Fatalf("第 %d 条日志的 retry_round = %d，期望 %d", i, row.RetryRound, i)
		}
		if row.ConfirmChainID == nil || *row.ConfirmChainID != chainID {
			t.Fatalf("第 %d 条日志的 confirm_chain_id 不一致", i)
		}
	}

	var tokenCount int64
	if err := db.Model(&models.ConfirmToken{}).Count(&tokenCount).Error; err != nil {
		t.Fatalf("统计 token 失败: %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("期望仅创建 1 个 token，实际 %d", tokenCount)
	}

	time.Sleep(1500 * time.Millisecond)
	if got := countLogsByReminder(t, db, reminder.ID); got != 11 {
		t.Fatalf("达到最大重试后仍继续发送，日志数 = %d", got)
	}
}

func TestLogServiceListPaginatesByConfirmChain(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)
	reminder := createConfirmReminder(t, db, 1, 2)

	_, err := triggerDispatch(context.Background(), stack.dispatch, reminder)
	if err != nil {
		t.Fatalf("triggerDispatch 失败: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		return countLogsByReminder(t, db, reminder.ID) == 3
	}, "等待确认重试链生成完成")

	other := &models.DeliveryLog{
		ReminderID: reminder.ID,
		FiredAt:    models.TimeOf(time.Now().Add(time.Minute)),
		Title:      "另一条独立日志",
		Content:    "独立内容",
		Status:     "success",
		Source:     "web",
		RetryRound: 0,
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("创建独立日志失败: %v", err)
	}

	logs := listLogsByReminder(t, db, reminder.ID)
	if len(logs) != 4 {
		t.Fatalf("期望 4 条原始日志，实际 %d", len(logs))
	}
	chainID := ""
	for _, row := range logs {
		if row.ConfirmChainID != nil && *row.ConfirmChainID != "" {
			chainID = *row.ConfirmChainID
			break
		}
	}
	if chainID == "" {
		t.Fatal("确认链 ID 为空")
	}

	svc := NewLogService(db)
	items, total, err := svc.List(LogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("期望分页总数为 2，实际 %d", total)
	}
	if len(items) != 4 {
		t.Fatalf("期望返回 4 条展示日志，实际 %d", len(items))
	}
	if items[0].ID != other.ID {
		t.Fatalf("第一页首条应为独立日志，实际 %d", items[0].ID)
	}
	if items[1].RetryRound != 0 {
		t.Fatalf("确认链主日志 retry_round 应为 0，实际 %d", items[1].RetryRound)
	}
	if items[1].ConfirmChainID == nil || *items[1].ConfirmChainID != chainID {
		t.Fatal("确认链主日志链 ID 不匹配")
	}
	if items[2].RetryRound != 1 || items[3].RetryRound != 2 {
		t.Fatalf("重发日志顺序不正确，实际为 %d、%d", items[2].RetryRound, items[3].RetryRound)
	}

	page2, total2, err := svc.List(LogFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if total2 != 2 {
		t.Fatalf("分页总数应保持为 2，实际 %d", total2)
	}
	if len(page2) != 3 {
		t.Fatalf("第二页应返回整条确认链 3 条展示日志，实际 %d", len(page2))
	}
	if page2[0].RetryRound != 0 || page2[1].RetryRound != 1 || page2[2].RetryRound != 2 {
		t.Fatalf("第二页确认链顺序不正确：%d、%d、%d", page2[0].RetryRound, page2[1].RetryRound, page2[2].RetryRound)
	}
}

func TestConfirmRetryStopsAfterConfirmAndMarksWholeChain(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)
	reminder := createConfirmReminder(t, db, 1, 10)

	logID, err := triggerDispatch(context.Background(), stack.dispatch, reminder)
	if err != nil {
		t.Fatalf("triggerDispatch 失败: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		return countLogsByReminder(t, db, reminder.ID) >= 3
	}, "等待至少 2 次重试")

	first := getLogByID(t, db, logID)
	if first.ConfirmChainID == nil || *first.ConfirmChainID == "" {
		t.Fatal("首条日志缺少 confirm_chain_id")
	}
	chainID := *first.ConfirmChainID
	token := getTokenByChain(t, db, chainID)

	if _, err := stack.confirmSvc.ConsumeToken(token.Token); err != nil {
		t.Fatalf("消费确认 token 失败: %v", err)
	}

	countBefore := countLogsByReminder(t, db, reminder.ID)
	waitFor(t, 2*time.Second, func() bool {
		return !stack.hasTimer(chainID)
	}, "等待确认链 timer 被清理")

	time.Sleep(1500 * time.Millisecond)
	countAfter := countLogsByReminder(t, db, reminder.ID)
	if countAfter != countBefore {
		t.Fatalf("确认后仍继续发送，确认前后日志数 %d -> %d", countBefore, countAfter)
	}

	logs := listLogsByChain(t, db, chainID)
	if len(logs) == 0 {
		t.Fatal("确认链日志为空")
	}
	for i, row := range logs {
		if !row.Confirmed {
			t.Fatalf("第 %d 条链路日志未标记 confirmed", i)
		}
		if row.ConfirmedAt == nil {
			t.Fatalf("第 %d 条链路日志缺少 confirmed_at", i)
		}
	}

	token = getTokenByChain(t, db, chainID)
	if token.UsedAt == nil {
		t.Fatal("确认后 token.used_at 未写入")
	}
}

func TestToggleDisablesActiveConfirmChain(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)
	reminder := createConfirmReminder(t, db, 1, 10)

	logID, err := triggerDispatch(context.Background(), stack.dispatch, reminder)
	if err != nil {
		t.Fatalf("triggerDispatch 失败: %v", err)
	}

	waitFor(t, 5*time.Second, func() bool {
		return countLogsByReminder(t, db, reminder.ID) >= 2
	}, "等待首轮重试")

	first := getLogByID(t, db, logID)
	if first.ConfirmChainID == nil || *first.ConfirmChainID == "" {
		t.Fatal("首条日志缺少 confirm_chain_id")
	}
	chainID := *first.ConfirmChainID
	countBefore := countLogsByReminder(t, db, reminder.ID)

	view, err := stack.reminderSvc.Toggle(reminder.ID)
	if err != nil {
		t.Fatalf("禁用提醒失败: %v", err)
	}
	if view.Enabled {
		t.Fatal("提醒应已被禁用")
	}

	waitFor(t, 2*time.Second, func() bool {
		return !stack.hasTimer(chainID)
	}, "等待禁用后确认链 timer 被清理")

	time.Sleep(1500 * time.Millisecond)
	countAfter := countLogsByReminder(t, db, reminder.ID)
	if countAfter != countBefore {
		t.Fatalf("禁用提醒后仍继续发送，日志数 %d -> %d", countBefore, countAfter)
	}
}

// chainSeed 描述一条待造的确认链。
type chainSeed struct {
	chainID        string
	rounds         []int
	lastFiredAgo   time.Duration
	tokenTTL       time.Duration
	confirmedRound *int
}

func intPtr(v int) *int { return &v }

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

func TestRecoverChains_OnlyNewestChainPerReminder(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)

	seedChain(t, db, r.ID, chainSeed{
		chainID:      "chain-old",
		rounds:       []int{0, 1},
		lastFiredAgo: 2 * time.Hour,
		tokenTTL:     48 * time.Hour,
	})
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

func TestRecoverChains_SkipsExhaustedChain(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 2)
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

func TestRecoverChains_FiresImmediatelyWhenIntervalAlreadyPassed(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
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

func TestRecoverChains_WaitsRemainingInterval(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 5, 5)
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

func TestRecoverChains_QueryCountIndependentOfChainCount(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	recoverWith := func(scoped *gorm.DB) int {
		mgr := NewConfirmRetryManager(scoped, stack.dispatch, stack.confirmSvc, testTZ(cstZone()))
		n, err := mgr.RecoverChains()
		if err != nil {
			t.Fatalf("RecoverChains 失败: %v", err)
		}
		mgr.StopAll()
		return n
	}

	r1 := createConfirmReminder(t, db, 60, 5)
	seedChain(t, db, r1.ID, chainSeed{
		chainID: "chain-q1", rounds: []int{0}, lastFiredAgo: 0, tokenTTL: time.Hour,
	})
	queriesOne := countQueries(t, db, func(scoped *gorm.DB) { recoverWith(scoped) })

	for i := 0; i < 20; i++ {
		ri := createConfirmReminder(t, db, 60, 5)
		seedChain(t, db, ri.ID, chainSeed{
			chainID:      "chain-q-" + itoaTest(i),
			rounds:       []int{0},
			lastFiredAgo: 0,
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

func TestConfirmRetryCancel(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)

	chainID := "chain-cancel"
	stack.confirmMgr.Schedule(r, chainID, 0)

	if !stack.hasTimer(chainID) {
		t.Fatal("应已注册定时器")
	}

	stack.confirmMgr.Cancel(chainID)

	if stack.hasTimer(chainID) {
		t.Fatal("Cancel 后定时器应被移除")
	}

	stack.confirmMgr.Cancel("no-such-chain")
}

func TestConfirmRetryRetry_StopsWhenConfirmDisabled(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
	chainID := "chain-noconfirm"

	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: models.TimeNow(), Title: "t", Status: "success",
		Source: "web", ConfirmChainID: &chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	if _, err := stack.confirmSvc.CreateToken(dl.ID, time.Hour); err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	if err := db.Model(&models.Reminder{}).Where("id = ?", r.ID).
		Update("require_confirm", false).Error; err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	before := countLogsByReminder(t, db, r.ID)
	stack.confirmMgr.retry(r.ID, chainID, 1)
	after := countLogsByReminder(t, db, r.ID)

	if after != before {
		t.Fatalf("关闭确认后不应再重发，日志 %d -> %d", before, after)
	}
	if stack.hasTimer(chainID) {
		t.Fatal("应停止链，不应保留定时器")
	}
}

func TestConfirmRetryRetry_StopsWhenReminderDeleted(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
	const chainID = "chain-deleted"

	if err := db.Unscoped().Delete(&models.Reminder{}, r.ID).Error; err != nil {
		t.Fatalf("删除提醒失败: %v", err)
	}

	stack.confirmMgr.retry(r.ID, chainID, 1)

	if stack.hasTimer(chainID) {
		t.Fatal("应停止链")
	}
}

func TestConfirmRetryRetry_StopsWhenTokenExpired(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
	chainID := "chain-expired"

	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: models.TimeNow(), Title: "t", Status: "success",
		Source: "web", ConfirmChainID: &chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	if _, err := stack.confirmSvc.CreateToken(dl.ID, -time.Hour); err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	before := countLogsByReminder(t, db, r.ID)
	stack.confirmMgr.retry(r.ID, chainID, 1)
	after := countLogsByReminder(t, db, r.ID)

	if after != before {
		t.Fatalf("令牌过期后不应重发，日志 %d -> %d", before, after)
	}
}

func TestConfirmRetryRetry_StopsWhenAlreadyConfirmed(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
	chainID := "chain-confirmed"

	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: models.TimeNow(), Title: "t", Status: "success",
		Source: "web", ConfirmChainID: &chainID, Confirmed: true,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	if _, err := stack.confirmSvc.CreateToken(dl.ID, time.Hour); err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	before := countLogsByReminder(t, db, r.ID)
	stack.confirmMgr.retry(r.ID, chainID, 1)
	after := countLogsByReminder(t, db, r.ID)

	if after != before {
		t.Fatalf("已确认后不应重发，日志 %d -> %d", before, after)
	}
}

func TestConfirmRetryRetry_StopsExceedingMaxRetries(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 2)
	chainID := "chain-max"

	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: models.TimeNow(), Title: "t", Status: "success",
		Source: "web", ConfirmChainID: &chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	if _, err := stack.confirmSvc.CreateToken(dl.ID, time.Hour); err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	before := countLogsByReminder(t, db, r.ID)
	stack.confirmMgr.retry(r.ID, chainID, 10)
	after := countLogsByReminder(t, db, r.ID)

	if after != before {
		t.Fatalf("超过最大重试次数不应重发，日志 %d -> %d", before, after)
	}
}

func TestConfirmRetryCancelByReminderID_NoChains(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	stack.confirmMgr.CancelByReminderID(0)
	stack.confirmMgr.CancelByReminderID(9999)
}

func TestConfirmRetryStopAll(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)
	stack.confirmMgr.Schedule(r, "c1", 0)
	stack.confirmMgr.Schedule(r, "c2", 0)

	stack.confirmMgr.StopAll()

	if stack.hasTimer("c1") || stack.hasTimer("c2") {
		t.Fatal("StopAll 应清空所有定时器")
	}
}

func TestConfirmConsumeToken_DBError(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfirmService(db, testCfg())

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	_ = sqlDB.Close()

	if _, err := svc.ConsumeToken("any"); err == nil {
		t.Fatal("DB 关闭后应报错")
	}
}
