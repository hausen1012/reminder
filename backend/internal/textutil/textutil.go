// Package textutil 提供日志与消息处理共用的文本工具：URL 脱敏与按字符截断。
package textutil

import "net/url"

// RedactURL 抹掉 URL 的 userinfo、query 取值与 fragment，只保留协议、主机、路径与参数名。
//
// 例如 https://oapi.dingtalk.com/robot/send?access_token=abc&sign=def
// → https://oapi.dingtalk.com/robot/send?access_token=***&sign=***
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	safe := *u
	if safe.User != nil {
		safe.User = url.User("***")
	}
	if safe.RawQuery != "" {
		q := safe.Query()
		for k := range q {
			q.Set(k, "***")
		}
		safe.RawQuery = q.Encode()
	}
	safe.Fragment = ""
	safe.RawFragment = ""
	return safe.String()
}

// RedactURLString 是 RedactURL 的字符串入参版本；解析失败时返回 "***"。
func RedactURLString(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "***"
	}
	return RedactURL(u)
}

// RedactQuery 抹掉 query string 里的取值，只保留参数名；空串返回空串，解析失败返回 "***"。
func RedactQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "***"
	}
	for k := range q {
		q.Set(k, "***")
	}
	return q.Encode()
}

// TruncateRunes 按字符截断，避免截出半个汉字；超长时追加省略号。
func TruncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
