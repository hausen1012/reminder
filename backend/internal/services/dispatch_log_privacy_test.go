package services

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/notifier"
	"gorm.io/gorm"
)

// captureLogs 把全局 slog 临时重定向到内存 buffer，t.Cleanup 时还原。
//
// slog.SetDefault 是进程级全局状态，因此本文件里的用例都不要标记 t.Parallel。
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// 投递日志不能泄露确认链接。
//
// vars 里带 confirm_url，拼出来就是完整可用的确认令牌；
// 谁读到日志谁就能替用户把提醒确认掉，因此绝不能出现在日志里。
// 这里刻意不用 log 通道——它的职责就是把正文打进日志，会掩盖要验证的点。
func TestDispatchLogDoesNotLeakConfirmURL(t *testing.T) {
	logs := captureLogs(t)

	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	db := openTestDB(t)
	stack := newConfirmTestStack(t, db)
	reminder := createConfirmReminderWithWebhook(t, db, 60, 1, sink.URL)

	if _, err := triggerDispatch(context.Background(), stack.dispatch, reminder); err != nil {
		t.Fatalf("触发投递失败: %v", err)
	}

	var tok models.ConfirmToken
	if err := db.First(&tok).Error; err != nil {
		t.Fatalf("确认令牌未创建: %v", err)
	}
	if len(tok.Token) < 32 {
		t.Fatalf("令牌长度异常: %q", tok.Token)
	}

	out := logs.String()
	if strings.Contains(out, tok.Token) {
		t.Fatalf("日志里出现了确认令牌明文:\n%s", out)
	}
	// 变量数量这类"形状信息"仍应保留，便于排查
	if !strings.Contains(out, "vars_count") {
		t.Fatalf("日志缺少 vars_count 字段，排查信息丢失:\n%s", out)
	}
}

// webhook 通道的日志不能带 Authorization 等 header 取值。
func TestWebhookLogRedactsCredentials(t *testing.T) {
	logs := captureLogs(t)

	const secretValue = "super-secret-token-value"
	var gotAuth string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	// WebhookConfig.AuthorizationEnc 在传入 Notifier 时已是明文
	// （ChannelService 负责落库加密与读出解密），这里直接给明文。
	cfg, _ := json.Marshal(map[string]any{
		"method":            "POST",
		"url":               sink.URL + "/hook?access_token=query-secret",
		"authorization_enc": "Bearer " + secretValue,
	})

	n, err := notifier.Get("webhook")
	if err != nil {
		t.Fatalf("获取 webhook 发送器失败: %v", err)
	}
	if err := n.Send(context.Background(), cfg, notifier.Message{Subject: "s", Body: "b"}); err != nil {
		t.Fatalf("webhook 发送失败: %v", err)
	}

	// 凭据必须真的发出去（功能没被削弱）
	if gotAuth != "Bearer "+secretValue {
		t.Fatalf("Authorization 未正确发送: %q", gotAuth)
	}

	out := logs.String()
	for _, leak := range []string{secretValue, "query-secret"} {
		if strings.Contains(out, leak) {
			t.Fatalf("日志里出现了凭据 %q:\n%s", leak, out)
		}
	}
}

// createConfirmReminderWithWebhook 用 webhook 通道创建"需要确认"的提醒。
func createConfirmReminderWithWebhook(t *testing.T, db *gorm.DB, intervalSec, maxRetries int, url string) *models.Reminder {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{"method": "POST", "url": url})
	channel := &models.Channel{
		Name:    "webhook-" + time.Now().Format("150405.000000"),
		Type:    "webhook",
		Enabled: true,
		Config:  cfg,
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
