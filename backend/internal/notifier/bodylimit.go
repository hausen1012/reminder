// 消息正文长度约束：超长时按 Spec.MaxBodyLen 截断并追加省略号。
package notifier

import "strings"

// TruncatedSuffix 是截断后追加的省略号。
const TruncatedSuffix = "…"

// LimitFor 按通道类型返回可用的正文长度上限；0 表示不限。
func LimitFor(typ string) int {
	spec, ok := SpecOf(typ)
	if !ok {
		return 0
	}
	return spec.MaxBodyLen
}

// LimitBody 按通道类型的上限截断正文；未超长时原样返回。
//
// 按 rune 计数而非字节，避免把中文截在半个字符上。
func LimitBody(typ string, msg Message) Message {
	limit := LimitFor(typ)
	if limit <= 0 {
		return msg
	}
	runes := []rune(msg.Body)
	if len(runes) <= limit {
		return msg
	}
	// 省略号本身占一位，截断长度取 limit-1
	msg.Body = strings.TrimRight(string(runes[:limit-1]), " \n\t") + TruncatedSuffix
	if msg.Vars != nil {
		// Vars 里的 content 与正文同源，同步截断，避免 webhook 模板引用时长度不一致
		msg.Vars = cloneVars(msg.Vars)
		msg.Vars["content"] = msg.Body
	}
	return msg
}

func cloneVars(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
