package services

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/config"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// newConfirmSvc 构造确认服务。
func newConfirmSvc(t *testing.T, baseURL string) (*ConfirmService, *gorm.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewConfirmService(db, &config.Config{BaseURL: baseURL}), db
}

// seedLogWithChain 插入一条可用于确认的投递日志。
func seedLogWithChain(t *testing.T, db *gorm.DB, chainID *string) *models.DeliveryLog {
	t.Helper()
	dl := &models.DeliveryLog{
		ReminderID:     1,
		FiredAt:        time.Now(),
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
		ReminderID: 1, FiredAt: time.Now(), Title: "重发", Status: "success",
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

	if _, err := svc.ConsumeToken(token); err == nil {
		t.Fatal("日志不存在时应报错")
	}
}

// BuildURL 应拼接 base/c/token，base 为空时用默认值。
func TestConfirmBuildURL(t *testing.T) {
	db := openTestDB(t)

	svc := NewConfirmService(db, &config.Config{BaseURL: "https://example.com"})
	if got := svc.BuildURL("abc"); got != "https://example.com/c/abc" {
		t.Fatalf("BuildURL = %q", got)
	}

	empty := NewConfirmService(db, &config.Config{BaseURL: ""})
	if got := empty.BuildURL("abc"); got == "" || !containsSub(got, "/c/abc") {
		t.Fatalf("base 为空时应使用默认值，实际 %q", got)
	}
}
