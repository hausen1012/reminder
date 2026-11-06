// Package llm 提供与厂商无关的对话接口，以及 Anthropic / OpenAI 兼容协议两个实现。
//
// 上层（services.AIService）只面向本包的 Turn / ToolCall / ToolResult 与 Provider 接口，
// 新增厂商时只需实现 Provider，无需改动工具编排逻辑。
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

// Turn 是会话中的一轮。
//
// 刻意不用 "assistant 消息里嵌 tool_result" 这种厂商特有的形态：
//   - Role=user 且 ToolResults 为空：普通用户文本（Text）
//   - Role=assistant：模型输出，可能同时带文本与工具调用
//   - Role=user 且 ToolResults 非空：回填工具结果
//
// 各 provider 负责把它翻译成自家协议（Anthropic 的 tool_result 也在 user 轮里，
// OpenAI 兼容协议则是独立的 role=tool 消息）。
type Turn struct {
	Role        string
	Text        string
	ToolCalls   []ToolCall
	ToolResults []ToolResult
	// Raw 是上一轮模型响应的厂商原始结构，用于逐字回放。
	//
	// Anthropic 在开启思考并要求工具调用时，要求把 thinking 块与签名原样传回，
	// 只靠 Text/ToolCalls 重建会丢掉签名。由 Provider 自己写入与解释，
	// 上层编排逻辑只需原样带回来。
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

// Reply 是模型的输出：文本 + 可能存在的工具调用。
//
// 文本与工具调用可以同时出现（模型先说一句话再调工具），调用方应两者都保留。
type Reply struct {
	Text      string
	ToolCalls []ToolCall
	// Truncated 表示回复因达到 max_tokens 被截断（Anthropic 的 max_tokens、
	// OpenAI 的 length）。此时工具调用可能只传了一半，调用方应停止本轮对话，
	// 而不是继续执行残缺的工具参数。
	Truncated bool
	// Raw 是厂商原始响应，交给上层塞进下一次请求的 Turn.Raw 逐字回放。
	Raw any
}

// Provider 是厂商适配接口。
//
// 实现必须把 ctx 透传到底层 HTTP 请求，以便超时/取消生效。
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
