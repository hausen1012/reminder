package services

import (
	"context"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
)

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
