package notifier

// plainText 把消息正文降级成纯文本：html 去标签，markdown 去标记，text 原样。
//
// 各通道对富文本的支持程度不同，凡是只收纯文本的通道都走这里，避免每个通道各写一份 switch。
func plainText(msg Message) string {
	switch msg.Format {
	case "html":
		return StripHTML(msg.Body)
	case "markdown":
		return StripMarkdown(msg.Body)
	default:
		return msg.Body
	}
}

// plainTextWithSubject 返回「标题 + 换行 + 正文（纯文本）」；标题为空时只返回正文。
func plainTextWithSubject(msg Message) string {
	body := plainText(msg)
	if msg.Subject == "" {
		return body
	}
	return msg.Subject + "\n" + body
}
