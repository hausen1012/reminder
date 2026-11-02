package llm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// AnthropicProvider 基于官方 Go SDK 调用 Claude。
//
// 走 beta 消息接口：官方地址下额外带上服务端拒答回退（fallbacks: default），
// 用自建/中转 base_url 时不带任何 beta 头，避免对端不认识而报错。
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
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
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
		// 显式用自适应思考：Opus 5.5 无法关闭思考，这里的唯一作用是让
		// 思考块在回放时保持原样（见 toMessageParam 的 Raw 分支）。
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
		return nil, err
	}

	reply := &Reply{Raw: resp.ToParam()}
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
