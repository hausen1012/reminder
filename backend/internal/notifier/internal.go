package notifier

import "encoding/base64"

// base64Encode 提供给 SMTP MIME header 编码使用，封装一层方便复用。
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
