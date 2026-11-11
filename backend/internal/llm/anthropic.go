package llm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/reminder/backend/internal/logger"
)

// AnthropicProvider 基于官方 Go SDK 调用 Claude。
// 官方地址带 fallbacks: default beta 头，自定义 base_url 时不带。
type AnthropicProvider struct {
	client anthropic.Client
	model  string
	// official 表示使用官方地址，可用 beta 能力。
	official bool
}

// NewAnthropicProvider 构造 Anthropic Provider。
func NewAnthropicProvider(baseURL, apiKey, model string) *AnthropicProvider {
	if strings.TrimSpace(model) == "" {
		model = "claude-opus-5-5"
	}
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		// 关掉 SDK 内置的自动重试：重试次数由上层 ChatWithRetry 按设置页配置统一控制，
		// 两处都重试会让「最大重试次数」实际生效的次数被放大数倍。
		option.WithMaxRetries(0),
	}
	official := strings.TrimSpace(baseURL) == ""
	if !official {
		opts = append(opts, option.WithBaseURL(strings.TrimRight(baseURL, "/")))
	}
	return &AnthropicProvider{
		client:   anthropic.NewClient(opts...),
		model:    model,
		official: official,
	}
}

// Chat 发起一次对话。
func (p *AnthropicProvider) Chat(ctx context.Context, req Request) (*Reply, error) {
	msgs := make([]anthropic.BetaMessageParam, 0, len(req.Turns))
	for _, t := range req.Turns {
		msgs = append(msgs, p.toMessageParam(t))
	}

	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: req.MaxTokens,
		Messages:  msgs,
		// 启用自适应思考以支持思考块原样回放
		Thinking: anthropic.BetaThinkingConfigParamUnion{
			OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{},
		},
	}
	if params.MaxTokens <= 0 {
		params.MaxTokens = 4096
	}
	if strings.TrimSpace(req.System) != "" {
		params.System = []anthropic.BetaTextBlockParam{{Text: req.System}}
	}
	if len(req.Tools) > 0 {
		params.Tools = make([]anthropic.BetaToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			params.Tools = append(params.Tools, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.BetaToolInputSchemaParam{
					Properties: t.InputSchema["properties"],
					Required:   requiredOf(t.InputSchema),
				},
			}})
		}
		params.ToolChoice = anthropic.BetaToolChoiceUnionParam{
			OfAuto: &anthropic.BetaToolChoiceAutoParam{},
		}
	}
	if p.official {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}

	resp, err := p.client.Beta.Messages.New(ctx, params)
	if err != nil {
		// 错误里的状态码、请求 id 与上游文案是排查限流/鉴权的关键；
		// logger.FromContext 会带上本次面板请求的 request_id。
		attrs := []any{
			"provider", "anthropic", "model", p.model,
			"official", p.official, "msgs", len(msgs), "tools", len(req.Tools),
		}
		attrs = append(attrs, ErrorAttrs(err)...)
		logger.FromContext(ctx).Warn("AI 请求返回错误", attrs...)
		return nil, err
	}

	reply := &Reply{
		Raw: resp.ToParam(),
		// stop_reason=max_tokens 表示被输出上限截断：工具块可能不完整，上层应停止本轮
		Truncated: resp.StopReason == anthropic.BetaStopReasonMaxTokens,
	}
	logger.FromContext(ctx).Debug("AI 请求完成",
		"provider", "anthropic", "model", p.model,
		"stop_reason", string(resp.StopReason),
		"input_tokens", resp.Usage.InputTokens, "output_tokens", resp.Usage.OutputTokens,
		"cache_read_tokens", resp.Usage.CacheReadInputTokens,
		"truncated", reply.Truncated)
	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.BetaTextBlock:
			reply.Text += v.Text
		case anthropic.BetaToolUseBlock:
			input, _ := json.Marshal(v.Input)
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{
				ID:    v.ID,
				Name:  v.Name,
				Input: input,
			})
		}
	}
	return reply, nil
}

// toMessageParam 把一轮对话翻译成 SDK 结构。
//
// assistant 轮若带 Raw（上一轮的原始响应），原样回放：思考块与签名必须逐字保留，
// 否则工具调用循环会因 thinking 块缺失或失真被拒绝。
func (p *AnthropicProvider) toMessageParam(t Turn) anthropic.BetaMessageParam {
	if t.Role == "assistant" && t.Raw != nil {
		if m, ok := t.Raw.(anthropic.BetaMessageParam); ok {
			return m
		}
	}
	if len(t.ToolResults) > 0 {
		blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(t.ToolResults))
		for _, r := range t.ToolResults {
			blocks = append(blocks, anthropic.NewBetaToolResultBlock(r.CallID, r.Content, r.IsError))
		}
		return anthropic.NewBetaUserMessage(blocks...)
	}
	if t.Role == "assistant" {
		blocks := make([]anthropic.BetaContentBlockParamUnion, 0, 1+len(t.ToolCalls))
		if t.Text != "" {
			blocks = append(blocks, anthropic.NewBetaTextBlock(t.Text))
		}
		for _, c := range t.ToolCalls {
			var input any
			_ = json.Unmarshal(c.Input, &input)
			blocks = append(blocks, anthropic.NewBetaToolUseBlock(c.ID, input, c.Name))
		}
		return anthropic.BetaMessageParam{OfAssistant: &anthropic.BetaAssistantMessageParam{Content: blocks}}
	}
	return anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(t.Text))
}

// requiredOf 从 JSON schema 中取出 required 列表。
func requiredOf(schema map[string]any) []string {
	raw, ok := schema["required"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// 静态断言：AnthropicProvider 实现 Provider。
var _ Provider = (*AnthropicProvider)(nil)
