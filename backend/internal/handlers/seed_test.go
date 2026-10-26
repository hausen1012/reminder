package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// seedLog 插入一条投递日志用于测试，返回其 ID。
// retryRound > 0 表示确认链上的重发记录。
func seedLog(t *testing.T, db *gorm.DB, title string, retryRound int, chainID *string) uint {
	t.Helper()
	dl := &models.DeliveryLog{
		ReminderID:     1,
		FiredAt:        time.Now().Add(-time.Duration(retryRound) * time.Minute),
		Title:          title,
		Content:        "正文",
		Status:         "success",
		Source:         "web",
		RetryRound:     retryRound,
		ConfirmChainID: chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("插入测试日志失败: %v", err)
	}
	return dl.ID
}

// seedAttempt 为指定日志插入一条投递尝试记录。
func seedAttempt(t *testing.T, db *gorm.DB, deliveryLogID uint, status string) {
	t.Helper()
	a := &models.DeliveryAttempt{
		DeliveryLogID: deliveryLogID,
		ChannelID:     1,
		ChannelType:   "log",
		ChannelName:   fmt.Sprintf("ch-%d", time.Now().UnixNano()),
		Attempt:       1,
		Status:        status,
	}
	if err := db.Create(a).Error; err != nil {
		t.Fatalf("插入测试 attempt 失败: %v", err)
	}
}
