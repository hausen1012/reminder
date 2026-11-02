package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// OpenAI 兼容实现：请求体形状与工具调用解析。
func TestOpenAIProvider_Chat_SendsToolsAndParsesToolCalls(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("路径 = %q，期望 /v1/chat/completions", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("请求体不是合法 JSON: %v", err)
		}
		_, _ = w.Write([]byte(`{
			"choices":[{
				"message":{
					"role":"assistant",
					"content":"先看看通道",
					"tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_channels","arguments":"{\"x\":1}"}}]
				},
				"finish_reason":"tool_calls"
			}]
		}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "sk-test", "deepseek-chat")
	reply, err := p.Chat(context.Background(), Request{
		System: "系统提示",
		Turns:  []Turn{{Role: "user", Text: "明天早上九点提醒我"}},
		Tools: []Tool{{
			Name:        "list_channels",
			Description: "列出通道",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		}},
		MaxTokens: 512,
	})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}

	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["model"] != "deepseek-chat" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("消息数 = %d，期望 system + user", len(msgs))
	}
	if gotBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice = %v", gotBody["tool_choice"])
	}

	if reply.Text != "先看看通道" {
		t.Fatalf("reply.Text = %q", reply.Text)
	}
	if len(reply.ToolCalls) != 1 {
		t.Fatalf("工具调用数 = %d", len(reply.ToolCalls))
	}
	if reply.ToolCalls[0].Name != "list_channels" || string(reply.ToolCalls[0].Input) != `{"x":1}` {
		t.Fatalf("工具调用解析错误: %+v", reply.ToolCalls[0])
	}
}

// 工具结果要展开成多条 role=tool 消息，assistant 轮带 tool_calls。
func TestOpenAIProvider_ExpandsToolResults(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"完成"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "", "local-model")
	_, err := p.Chat(context.Background(), Request{
		Turns: []Turn{
			{Role: "user", Text: "看看通道"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "list_channels", Input: json.RawMessage(`{}`)}}},
			{Role: "user", ToolResults: []ToolResult{{CallID: "c1", Name: "list_channels", Content: `{"channels":[]}`}}},
		},
		MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("消息数 = %d，期望 3（user/assistant/tool）", len(msgs))
	}
	asst, _ := msgs[1].(map[string]any)
	if asst["role"] != "assistant" || asst["tool_calls"] == nil {
		t.Fatalf("assistant 消息缺少 tool_calls: %v", asst)
	}
	toolMsg, _ := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "c1" {
		t.Fatalf("tool 消息不正确: %v", toolMsg)
	}
}

// 4xx 应返回带对端消息的错误，而不是空错误。
func TestOpenAIProvider_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","type":"authentication_error"}}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "bad", "m")
	_, err := p.Chat(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "hi"}}})
	if err == nil {
		t.Fatal("应返回错误")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("错误信息未透出对端消息: %v", err)
	}
}

// base_url 的各种写法都应落到 /v1/chat/completions。
func TestNewOpenAIProvider_NormalizesBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "https://api.openai.com/v1"},
		{"https://api.deepseek.com", "https://api.deepseek.com/v1"},
		{"https://api.deepseek.com/", "https://api.deepseek.com/v1"},
		{"https://x.example/v1", "https://x.example/v1"},
		{"http://localhost:11434/v1/", "http://localhost:11434/v1"},
	}
	for _, c := range cases {
		if got := NewOpenAIProvider(c.in, "", "m").baseURL; got != c.want {
			t.Errorf("NewOpenAIProvider(%q).baseURL = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// 工具调用参数为空时补成 {}，避免下游 JSON 解析失败。
func TestOpenAIProvider_EmptyArguments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"c1","type":"function","function":{"name":"list_channels","arguments":""}}]}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider(srv.URL, "", "m")
	reply, err := p.Chat(context.Background(), Request{Turns: []Turn{{Role: "user", Text: "x"}}})
	if err != nil {
		t.Fatalf("Chat 失败: %v", err)
	}
	if string(reply.ToolCalls[0].Input) != "{}" {
		t.Fatalf("空参数应补成 {}，实际 %q", reply.ToolCalls[0].Input)
	}
}

// requiredOf 从 schema 中取 required。
func TestRequiredOf(t *testing.T) {
	if got := requiredOf(map[string]any{"required": []any{"a", "b", 1}}); len(got) != 2 || got[0] != "a" {
		t.Fatalf("requiredOf 解析错误: %v", got)
	}
	if got := requiredOf(map[string]any{}); got != nil {
		t.Fatalf("无 required 应返回 nil，实际 %v", got)
	}
}
