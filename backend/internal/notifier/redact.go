package notifier

import (
	"net/http"
	"net/url"
	"sort"
)

// 本文件集中处理"写入日志前脱敏"的工具函数。
//
// 背景：钉钉 / 企微的 webhook URL 自带 access_token、key 之类的凭据，
// Webhook 通道还支持自定义 Authorization 头。这些值一旦被 slog 原样打出来，
// 就等于把凭据落到日志文件和容器 stdout 上，违反"密钥与口令不得写进日志"的约定。
// 因此凡是要记录 URL 或 header 的地方，一律走这里的函数。

// redactURL 抹掉 URL 里的 query 取值与 userinfo，只保留协议、主机、路径与参数名。
//
// 例如 https://oapi.dingtalk.com/robot/send?access_token=abc&sign=def
// 输出 https://oapi.dingtalk.com/robot/send?access_token=***&sign=***。
// 保留参数名的意义在于排查时能看出"签名参数有没有拼上"，但看不出具体值。
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	safe := *u
	if safe.User != nil {
		// 带上 userinfo 的 URL 极少见，一旦有必然是凭据
		safe.User = url.User("***")
	}
	if safe.RawQuery != "" {
		q := safe.Query()
		for k := range q {
			q.Set(k, "***")
		}
		safe.RawQuery = q.Encode()
	}
	// Fragment 可能被用来塞 token，统一丢弃
	safe.Fragment = ""
	safe.RawFragment = ""
	return safe.String()
}

// redactURLString 是 redactURL 的字符串入参版本；解析失败时只返回主机部分。
func redactURLString(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "***"
	}
	return redactURL(u)
}

// headerKeys 返回 header 名的有序列表，用于日志。
//
// 只返回名字不返回值：Authorization、X-Api-Key 之类都是凭据，
// 而"配了哪些 header"本身对排查已经足够。
func headerKeys(h http.Header) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
