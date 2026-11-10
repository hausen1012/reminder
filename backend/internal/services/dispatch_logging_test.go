package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
)

// 投递失败与重试必须在服务端日志里留痕：只靠「用时/错误」落库，
// 排查时拿不到"试了几次、哪次是永久错误"这类关键信息。
func TestDispatchRun_LogsFailureChain(t *testing.T) {
	logs := captureLogs(t)

	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)
	// 三次重试，测试里不真的等待
	dispatch.RetryDelays = []time.Duration{0, 0, 0}

	badCh := createChannel(t, db, "unknown-type")
	r := &models.Reminder{
		Title: "全失败", Calendar: "solar", ScheduleType: "once", Enabled: true, Source: "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, badCh.ID)
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	out := logs.String()
	// 未注册的通道类型属于配置错误，应当以 ERROR 记录并带上通道与类型
	for _, want := range []string{
		"通道类型未注册",
		"type=unknown-type",
		"投递完成",
		"status=failed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("投递失败日志缺少 %q:\n%s", want, out)
		}
	}
}

// 没有绑定通道属于用户的配置遗漏：应留下可读的 Warn，而不是静默标记失败。
func TestDispatchRun_LogsNoChannels(t *testing.T) {
	logs := captureLogs(t)

	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)

	r := &models.Reminder{
		Title: "无通道", Calendar: "solar", ScheduleType: "once", Enabled: true, Source: "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	out := logs.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "没有可用的通道") {
		t.Fatalf("缺少无通道的 Warn 日志:\n%s", out)
	}
}

// 成功投递只留汇总日志；单通道成功属于调试细节，默认级别下不应出现。
func TestDispatchRun_SuccessLoggedOnceAtInfo(t *testing.T) {
	logs := captureLogs(t)

	db := openTestDB(t)
	dispatch := newDispatchStack(t, db)
	dispatch.RetryDelays = []time.Duration{0}

	okCh := createChannel(t, db, "log")
	r := &models.Reminder{
		Title: "成功", Content: "正文", Calendar: "solar", ScheduleType: "once", Enabled: true, Source: "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	bindChannels(t, db, r.ID, okCh.ID)
	logID := createPendingLog(t, db, r)

	dispatch.Run(context.Background(), r, logID)

	out := logs.String()
	if !strings.Contains(out, "status=success") {
		t.Fatalf("缺少成功汇总日志:\n%s", out)
	}
	// captureLogs 用 LevelDebug，因此这条只会以 DEBUG 出现：
	// 默认 info 级别下用户看不到它，避免高频成功明细淹没日志。
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "通道发送成功") && !strings.Contains(line, "level=DEBUG") {
			t.Fatalf("单通道成功应为 DEBUG 级别:\n%s", line)
		}
	}
	if strings.Contains(out, "level=ERROR") || strings.Contains(out, "level=WARN") {
		t.Fatalf("成功投递不应产生 Warn/Error 日志:\n%s", out)
	}
}
