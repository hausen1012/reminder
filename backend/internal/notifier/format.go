package notifier

import (
	"regexp"
	"strings"
)

var (
	htmlTagRe = regexp.MustCompile(`<[^>]*>`)
	// markdown 标记清理
	mdHeaderRe    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdBoldRe      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdItalicRe    = regexp.MustCompile(`\*(.+?)\*`)
	mdCodeRe      = regexp.MustCompile("`{1,3}[^`\n]+`{1,3}")
	mdLinkRe      = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	mdImageRe     = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`)
	mdBlockquoteRe = regexp.MustCompile(`(?m)^>\s+`)
	mdHrRe        = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
	mdListRe      = regexp.MustCompile(`(?m)^[\s]*[-*+]\s+`)
	mdNumListRe   = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
)

// StripHTML 去除所有 HTML 标签，返回纯文本。
func StripHTML(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}

// StripMarkdown 去除常见 Markdown 标记，返回纯文本。
func StripMarkdown(s string) string {
	result := s
	result = mdHeaderRe.ReplaceAllString(result, "")
	result = mdImageRe.ReplaceAllString(result, "$1")
	result = mdLinkRe.ReplaceAllString(result, "$1")
	result = mdBoldRe.ReplaceAllString(result, "$1")
	result = mdItalicRe.ReplaceAllString(result, "$1")
	result = mdCodeRe.ReplaceAllString(result, "")
	result = mdBlockquoteRe.ReplaceAllString(result, "")
	result = mdHrRe.ReplaceAllString(result, "")
	result = mdListRe.ReplaceAllString(result, "")
	result = mdNumListRe.ReplaceAllString(result, "")
	result = htmlTagRe.ReplaceAllString(result, "") // 安全兜底
	// 合并多余空白
	result = strings.Join(strings.Fields(result), " ")
	return strings.TrimSpace(result)
}