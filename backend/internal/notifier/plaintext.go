package notifier

// plainText 将 html / markdown 格式正文转换为纯文本。
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
