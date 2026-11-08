package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/reminder/backend/internal/logger"
)

// OpenAIProvider 用 OpenAI Chat Completions 协议对接任意兼容厂商（DeepSeek、Moonshot、
// 通义、Ollama、vLLM、中转站等），因此直接手写 HTTP 而不引入各家 SDK。
type OpenAIProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewOpenAIProvider 构造 OpenAI 兼容 Provider。
//
// baseURL 允许两种写法：以 /v1 结尾的完整前缀，或只填域名；
// 后者会自动补 /v1/chat/completions。
func NewOpenAIProvider(baseURL, apiKey, model string) *OpenAIProvider {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if !strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") {
		base += "/v1"
	}
	return &OpenAIProvider{
		baseURL: base,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content,omitempty"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

type oaRequest struct {
	Model      string      `json:"model"`
	Messages   []oaMessage `json:"messages"`
	Tools      []oaTool    `json:"tools,omitempty"`
	MaxTokens  int64       `json:"max_tokens,omitempty"`
	ToolChoice string      `json:"tool_choice,omitempty"`
}

type oaTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type oaResponse struct {
	Choices []struct {
		Message      oaMessage `json:"message"`
		FinishReason string    `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Chat 发起一次对话。
func (p *OpenAIProvider) Chat(ctx context.Context, req Request) (*Reply, error) {
	msgs := make([]oaMessage, 0, len(req.Turns)+1)
	if strings.TrimSpace(req.System) != "" {
		msgs = append(msgs, oaMessage{Role: "system", Content: req.System})
	}
	// OpenAI 协议要求 assistant 的 tool_calls 与随后每条 role=tool 一一对应，
	// 因此把 Turn 展开成多条消息。
	for _, t := range req.Turns {
		switch {
		case len(t.ToolResults) > 0:
			for _, r := range t.ToolResults {
				msgs = append(msgs, oaMessage{
					Role:       "tool",
					ToolCallID: r.CallID,
					Content:    r.Content,
				})
			}
		case t.Role == "assistant":
			m := oaMessage{Role: "assistant", Content: t.Text}
			for _, c := range t.ToolCalls {
				call := oaToolCall{ID: c.ID, Type: "function"}
				call.Function.Name = c.Name
				call.Function.Arguments = string(c.Input)
				m.ToolCalls = append(m.ToolCalls, call)
			}
			msgs = append(msgs, m)
		default:
			msgs = append(msgs, oaMessage{Role: "user", Content: t.Text})
		}
	}

	body := oaRequest{
		Model:     p.model,
		Messages:  msgs,
		MaxTokens: req.MaxTokens,
	}
	if body.MaxTokens <= 0 {
		body.MaxTokens = 4096
	}
	if len(req.Tools) > 0 {
		body.Tools = make([]oaTool, 0, len(req.Tools))
		for _, t := range req.Tools {
			var tool oaTool
			tool.Type = "function"
			tool.Function.Name = t.Name
			tool.Function.Description = t.Description
			tool.Function.Parameters = t.InputSchema
			body.Tools = append(body.Tools, tool)
		}
		body.ToolChoice = "auto"
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	reqURL := p.baseURL + "/chat/completions"
	started := time.Now()
	resp, err := p.client.Do(httpReq)
	if err != nil {
		logger.FromContext(ctx).Warn("AI 请求失败",
			"provider", "openai", "model", p.model, "url", redactURLString(reqURL),
			"msgs", len(msgs), "tools", len(body.Tools),
			"elapsed_ms", time.Since(started).Milliseconds(), "error", err)
		return nil, fmt.Errorf("请求 AI 服务失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var errOut oaResponse
		msg := truncate(respBody, 300)
		if err := json.Unmarshal(respBody, &errOut); err == nil && errOut.Error != nil && errOut.Error.Message != "" {
			msg = errOut.Error.Message
		}
		// 状态码与响应体是排查上游限流/鉴权问题的关键，这里完整留痕；
		// request_id 由 logger.FromContext 自动带上，能和面板请求对上。
		logger.FromContext(ctx).Warn("AI 请求返回错误",
			"provider", "openai", "model", p.model, "status", resp.StatusCode,
			"msgs", len(msgs), "tools", len(body.Tools),
			"elapsed_ms", time.Since(started).Milliseconds(),
			"body", truncate(respBody, 1000))
		return nil, &HTTPStatusError{
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("AI 服务返回错误（HTTP %d）: %s", resp.StatusCode, msg),
			Body:    truncate(respBody, 1000),
		}
	}

	var out oaResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		logger.FromContext(ctx).Error("解析 AI 响应失败",
			"provider", "openai", "model", p.model, "status", resp.StatusCode,
			"body", truncate(respBody, 1000), "error", err)
		return nil, fmt.Errorf("解析 AI 响应失败（HTTP %d）: %s", resp.StatusCode, truncate(respBody, 300))
	}
	if out.Error != nil {
		msg := truncate(respBody, 300)
		if out.Error.Message != "" {
			msg = out.Error.Message
		}
		return nil, &HTTPStatusError{
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("AI 服务返回错误（HTTP %d）: %s", resp.StatusCode, msg),
			Body:    truncate(respBody, 1000),
		}
	}
	if len(out.Choices) == 0 {
		logger.FromContext(ctx).Error("AI 服务未返回内容",
			"provider", "openai", "model", p.model, "body", truncate(respBody, 1000))
		return nil, fmt.Errorf("AI 服务未返回内容")
	}

	choice := out.Choices[0].Message
	// finish_reason=length 表示被 max_tokens 截断，工具参数可能不完整
	reply := &Reply{Text: choice.Content, Truncated: out.Choices[0].FinishReason == "length"}
	for _, c := range choice.ToolCalls {
		input := strings.TrimSpace(c.Function.Arguments)
		if input == "" {
			input = "{}"
		}
		reply.ToolCalls = append(reply.ToolCalls, ToolCall{
			ID:    c.ID,
			Name:  c.Function.Name,
			Input: json.RawMessage(input),
		})
	}
	logger.FromContext(ctx).Debug("AI 请求完成",
		"provider", "openai", "model", p.model,
		"elapsed_ms", time.Since(started).Milliseconds(),
		"tool_calls", len(reply.ToolCalls), "truncated", reply.Truncated,
		"prompt_tokens", out.Usage.PromptTokens, "completion_tokens", out.Usage.CompletionTokens)
	return reply, nil
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

var _ Provider = (*OpenAIProvider)(nil)
