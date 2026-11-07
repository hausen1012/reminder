package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/reminder/backend/internal/llm"
	"github.com/reminder/backend/internal/logger"
)

// AI 对话失败时必须留下「问了什么 + 上游怎么说的」：
// 只有一句「AI 调用失败」，事后既复现不了，也判断不了是限流还是参数错。
func TestAIChat_FailureLogsMessageAndUpstream(t *testing.T) {
	logs := captureLogs(t)

	svc, fake, _ := newAITestService(t)
	fake.err = &llm.HTTPStatusError{
		Status:  429,
		Message: "AI 服务返回错误（HTTP 429）: inference exceeds tpm/rpm limit",
		Body:    `{"error":{"message":"inference exceeds tpm/rpm limit"}}`,
	}

	_, err := chat(svc, "帮我建一个每天九点的提醒")
	if err == nil {
		t.Fatal("应返回错误")
	}

	out := logs.String()
	for _, want := range []string{
		"AI 对话开始",
		"AI 对话结束：调用失败",
		"user_message=帮我建一个每天九点的提醒",
		"status=429",
		"tpm/rpm limit",
		"round=1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("失败日志缺少 %q:\n%s", want, out)
		}
	}
}

// 成功对话留一条汇总：轮次、重试、草稿数，以及用户原话。
func TestAIChat_SuccessLogsSummary(t *testing.T) {
	logs := captureLogs(t)

	svc, _, _ := newAITestService(t, &llm.Reply{Text: "好的"})
	if _, err := chat(svc, "你好"); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}

	out := logs.String()
	for _, want := range []string{
		"AI 对话完成",
		"user_message=你好",
		"rounds=1",
		"retries=0",
		"drafts=0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("汇总日志缺少 %q:\n%s", want, out)
		}
	}
}

// 工具轮次的明细是 Debug：正常对话会跑好几轮，每轮一条 Info 会把日志刷满。
func TestAIChat_ToolRoundsAreDebug(t *testing.T) {
	logs := captureLogs(t)

	svc, _, _ := newAITestService(t,
		toolCall("list_channels", `{}`),
		&llm.Reply{Text: "通道列表如上"},
	)
	if _, err := chat(svc, "有哪些通道"); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}

	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "AI 工具轮次完成") && !strings.Contains(line, "level=DEBUG") {
			t.Fatalf("工具轮次明细应为 DEBUG:\n%s", line)
		}
	}
}

// 超长消息在日志里截断，避免一条日志刷出几千字。
func TestAIChat_TruncatesLongMessageInLogs(t *testing.T) {
	logs := captureLogs(t)

	svc, fake, _ := newAITestService(t)
	fake.err = errors.New("boom")

	long := strings.Repeat("长", 2000)
	_, _ = chat(svc, long)

	out := logs.String()
	if strings.Contains(out, long) {
		t.Fatal("日志不应包含未截断的长消息")
	}
	if !strings.Contains(out, "user_message=") || !strings.Contains(out, "…") {
		t.Fatalf("日志应包含截断后的消息（带省略号）:\n%s", out[:200])
	}
}

// 请求上下文里的 request_id 要出现在对话日志中，方便与访问日志对上。
func TestAIChat_LogsCarryRequestID(t *testing.T) {
	logs := captureLogs(t)

	svc, _, _ := newAITestService(t, &llm.Reply{Text: "好的"})
	ctx := logger.ContextWithRequestID(context.Background(), "req-42")
	if _, err := svc.Chat(ctx, ChatInput{Message: "你好"}); err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}

	if out := logs.String(); !strings.Contains(out, "request_id=req-42") {
		t.Fatalf("对话日志应带 request_id:\n%s", out)
	}
}
