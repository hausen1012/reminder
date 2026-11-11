package notifier

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var (
	// HTML 转纯文本用的匹配规则，顺序见 StripHTML
	htmlImgRe      = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	imgAltRe       = regexp.MustCompile(`(?i)\balt=["']([^"']*)["']`)
	imgSrcRe       = regexp.MustCompile(`(?i)\bsrc=["']([^"']*)["']`)
	htmlLinkRe     = regexp.MustCompile(`(?is)<a\s+[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	htmlBrRe       = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlLiCloseRe  = regexp.MustCompile(`(?i)</li>\s*`)
	htmlBlockEndRe = regexp.MustCompile(`(?i)</(?:p|div|h[1-6]|blockquote|pre)>`)
	htmlTagRe      = regexp.MustCompile(`<[^>]*>`)
	spaceRunRe     = regexp.MustCompile(`[ \t]+`)
)

// mdStripRenderer 把 markdown 转成 HTML 供纯文本降级使用。
//
// 启用 GFM 扩展（表格、删除线、自动链接）；WithUnsafe 让正文里的原始 HTML 原样透出，
// 由 StripHTML 统一处理，避免被当作注释丢弃。
var mdStripRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
)

// StripHTML 去除 HTML 标签，返回纯文本。
//
// 段落与列表保留换行，链接保留为「文本 (URL)」，图片取 alt（为空时取 src），实体字符会解码。
func StripHTML(s string) string {
	result := htmlImgRe.ReplaceAllStringFunc(s, imgAltText)
	result = htmlLinkRe.ReplaceAllStringFunc(result, linkText)
	result = htmlBrRe.ReplaceAllString(result, "\n")
	result = htmlLiCloseRe.ReplaceAllString(result, "\n")
	result = htmlBlockEndRe.ReplaceAllString(result, "\n\n")
	result = htmlTagRe.ReplaceAllString(result, "")
	result = html.UnescapeString(result)
	return cleanWhitespace(result)
}

// StripMarkdown 把 Markdown 渲染成 HTML 后再剥离标签，返回纯文本。
//
// 解析交给 goldmark，代码、链接、表格等边界的处理与渲染器保持一致。
func StripMarkdown(s string) string {
	var buf bytes.Buffer
	if err := mdStripRenderer.Convert([]byte(s), &buf); err != nil {
		return cleanWhitespace(s)
	}
	return StripHTML(buf.String())
}

// linkText 把 <a href="URL">文本</a> 转成「文本 (URL)」，文本与 URL 相同时只保留 URL。
func linkText(m string) string {
	sub := htmlLinkRe.FindStringSubmatch(m)
	if len(sub) < 3 {
		return m
	}
	url := strings.TrimSpace(sub[1])
	text := strings.TrimSpace(htmlTagRe.ReplaceAllString(sub[2], ""))
	if text == "" || text == url {
		return url
	}
	return text + " (" + url + ")"
}

// imgAltText 把 <img> 转成 alt 文本，alt 为空时退化为 src。
func imgAltText(m string) string {
	if alt := imgAltRe.FindStringSubmatch(m); len(alt) >= 2 && strings.TrimSpace(alt[1]) != "" {
		return strings.TrimSpace(alt[1])
	}
	if src := imgSrcRe.FindStringSubmatch(m); len(src) >= 2 {
		return strings.TrimSpace(src[1])
	}
	return ""
}

// cleanWhitespace 压缩空白：行内连续空格合并为一个，丢弃首尾空白，连续空行压缩为一个。
func cleanWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimSpace(spaceRunRe.ReplaceAllString(line, " "))
		if line == "" {
			blank++
			if blank <= 1 && len(out) > 0 {
				out = append(out, "")
			}
			continue
		}
		blank = 0
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// AppendLink 按内容格式把 url 作为链接追加到正文末尾。
//
// html 用 <a>，markdown 用 [text](url)，其余按纯文本追加。
func AppendLink(body, format, url, label string) string {
	switch format {
	case "html":
		return body + `<br><a href="` + url + `">` + label + `</a>`
	case "markdown":
		return body + "\n\n[" + label + "](" + url + ")"
	default:
		return body + "\n" + label + "：" + url
	}
}
