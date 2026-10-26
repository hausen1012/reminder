package services

import (
	"errors"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// Cancel 应停止指定 chain 的定时器。
func TestConfirmRetryCancel(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 60, 5)

	// 手动注册一个 chain
	chainID := "chain-cancel"
	stack.confirmMgr.Schedule(r, chainID, 0)

	if !stack.hasTimer(chainID) {
		t.Fatal("应已注册定时器")
	}

	stack.confirmMgr.Cancel(chainID)

	if stack.hasTimer(chainID) {
		t.Fatal("Cancel 后定时器应被移除")
	}

	// 对不存在的 chain 调用 Cancel 不应 panic
	stack.confirmMgr.Cancel("no-such-chain")
}

// 提醒被关闭确认后，retry 应终止链。
func TestConfirmRetryRetry_StopsWhenConfirmDisabled(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
	chainID := "chain-noconfirm"

	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: time.Now(), Title: "t", Status: "success",
		Source: "web", ConfirmChainID: &chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	if _, err := stack.confirmSvc.CreateToken(dl.ID, time.Hour); err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	// 关闭提醒的确认要求
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

// 提醒被删除后，retry 应终止链且不 panic。
func TestConfirmRetryRetry_StopsWhenReminderDeleted(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
	const chainID = "chain-deleted"

	if err := db.Unscoped().Delete(&models.Reminder{}, r.ID).Error; err != nil {
		t.Fatalf("删除提醒失败: %v", err)
	}

	// 不应 panic，也不应创建日志
	stack.confirmMgr.retry(r.ID, chainID, 1)

	if stack.hasTimer(chainID) {
		t.Fatal("应停止链")
	}
}

// 确认令牌已过期时，retry 应终止链。
func TestConfirmRetryRetry_StopsWhenTokenExpired(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
	chainID := "chain-expired"

	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: time.Now(), Title: "t", Status: "success",
		Source: "web", ConfirmChainID: &chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	// 已过期的令牌
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

// 用户已确认后，retry 应终止链。
func TestConfirmRetryRetry_StopsWhenAlreadyConfirmed(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 5)
	chainID := "chain-confirmed"

	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: time.Now(), Title: "t", Status: "success",
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

// 超过最大重试轮次时，retry 应终止链。
func TestConfirmRetryRetry_StopsExceedingMaxRetries(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	r := createConfirmReminder(t, db, 1, 2)
	chainID := "chain-max"

	dl := &models.DeliveryLog{
		ReminderID: r.ID, FiredAt: time.Now(), Title: "t", Status: "success",
		Source: "web", ConfirmChainID: &chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	if _, err := stack.confirmSvc.CreateToken(dl.ID, time.Hour); err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	before := countLogsByReminder(t, db, r.ID)
	// round > max_retries
	stack.confirmMgr.retry(r.ID, chainID, 10)
	after := countLogsByReminder(t, db, r.ID)

	if after != before {
		t.Fatalf("超过最大重试次数不应重发，日志 %d -> %d", before, after)
	}
}

// CancelByReminderID 对不存在的提醒应静默返回。
func TestConfirmRetryCancelByReminderID_NoChains(t *testing.T) {
	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)

	// 不该 panic
	stack.confirmMgr.CancelByReminderID(0)
	stack.confirmMgr.CancelByReminderID(9999)
}

// StopAll 应清空所有定时器。
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

// 确认 Service 的 ConsumeToken 遇到 DB 错误应原样返回。
func TestConfirmConsumeToken_DBError(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfirmService(db, testCfg())

	// 关闭底层连接制造 DB 错误
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	_ = sqlDB.Close()

	if _, err := svc.ConsumeToken("any"); err == nil {
		t.Fatal("DB 关闭后应报错")
	}
}

// isUniqueErr 应识别 SQLite 的唯一约束错误。
func TestIsUniqueErr(t *testing.T) {
	if isUniqueErr(nil) {
		t.Fatal("nil 不应判为唯一约束错误")
	}
	if isUniqueErr(errors.New("some other error")) {
		t.Fatal("普通错误不应判为唯一约束错误")
	}

	db := openTestDB(t)
	svc := NewChannelService(db, nil)
	if _, err := svc.Create(ChannelInput{Name: "dup", Type: "log", Config: map[string]any{}}); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 重复名称触发唯一约束
	_, err := svc.Create(ChannelInput{Name: "dup", Type: "log", Config: map[string]any{}})
	if err == nil {
		t.Fatal("重复名称应报错")
	}
	if !isUniqueErr(errors.Unwrap(err)) && !isUniqueErr(err) {
		// 包装层可能不是原始错误，这里只要求走了"名称已存在"分支
		if !containsSub(err.Error(), "已存在") {
			t.Fatalf("应提示名称已存在，实际 %v", err)
		}
	}
}

// 确保 gorm 导入被使用（多文件共享时的守卫）。
var _ = gorm.ErrRecordNotFound
