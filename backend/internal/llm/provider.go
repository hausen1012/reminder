// Package llm 提供统一的对话接口及 Anthropic、OpenAI 协议实现。
package llm

import (
	"context"
	"encoding/json"
)

// ToolCall 是模型请求调用某个工具的一次调用。
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult 是工具执行结果，回填给模型。
type ToolResult struct {
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// Turn 是会话中的一轮（用户文本、模型输出或工具执行结果）。
type Turn struct {
	Role        string
	Text        string
	ToolCalls   []ToolCall
	ToolResults []ToolResult
	// Raw 保存上一轮模型响应的原始结构，用于支持思考块等原样回放。
	Raw any
}

// Tool 是暴露给模型的工具定义。
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Request 是一次完整的对话请求。
type Request struct {
	System    string
	Turns     []Turn
	Tools     []Tool
	MaxTokens int64
}

// Reply 是模型的输出：文本 + 可能存在的工具调用，两者可同时出现。
type Reply struct {
	Text      string
	ToolCalls []ToolCall
	// Truncated 表示回复因达到 max_tokens 被截断。此时工具调用可能不完整，
	// 调用方应停止本轮对话。
	Truncated bool
	// Raw 是厂商原始响应，交给上层塞进下一次请求的 Turn.Raw 逐字回放。
	Raw any
}

// Provider 是厂商适配接口；实现必须把 ctx 透传到 HTTP 请求。
type Provider interface {
	Chat(ctx context.Context, req Request) (*Reply, error)
}

// Config 是构造 Provider 所需的配置。
type Config struct {
	Provider string // anthropic | openai
	BaseURL  string // 留空用厂商默认地址
	APIKey   string
	Model    string
}
