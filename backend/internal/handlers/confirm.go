// confirm 处理确认链接 GET /c/:token：验证令牌、标记已确认，返回内嵌的 HTML 状态页。
package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/services"
	"github.com/reminder/backend/internal/timezone"
)

// ConfirmHandler 处理确认链接。
type ConfirmHandler struct {
	Svc *services.ConfirmService
	// TZ 是站点时区：models.Time 以 UTC 落库，直接 Format 会让用户看到偏移时间
	TZ *timezone.Provider
}

// loc 返回用于展示的时间位置。
func (h *ConfirmHandler) loc() *time.Location {
	if h.TZ != nil {
		return h.TZ.Get()
	}
	return time.Local
}

// Confirm 处理 GET /c/:token。
//
// 幂等：已使用的令牌渲染成功页并注明确认时间，只有无效 / 过期 / 日志已清理才显示错误页。
func (h *ConfirmHandler) Confirm(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmErrorPage("确认链接无效")))
		return
	}

	dl, err := h.Svc.ConsumeToken(token)
	if err != nil {
		if errors.Is(err, services.ErrConfirmTokenUsed) && dl != nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmAlreadyPage(dl, h.loc())))
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmErrorPage(err.Error())))
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmSuccessPage(dl, h.loc())))
}

// formatInZone 把归一化为 UTC 的时间按站点时区格式化成 "2006-01-02 15:04:05"。
func formatInZone(t models.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return t.Std().In(loc).Format("2006-01-02 15:04:05")
}

// confirmSuccessPage 确认成功页
func confirmSuccessPage(dl *models.DeliveryLog, loc *time.Location) string {
	title := "已确认"
	if dl.Title != "" {
		title = dl.Title
	}
	confirmedAt := formatInZone(models.TimeNow(), loc)
	if dl.ConfirmedAt != nil {
		confirmedAt = formatInZone(*dl.ConfirmedAt, loc)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>确认成功</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;background:#f5f5f5;color:#333}
.card{background:#fff;border-radius:12px;padding:40px;text-align:center;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:420px;margin:20px}
.icon{width:56px;height:56px;background:#22c55e;border-radius:50%%;display:flex;align-items:center;justify-content:center;margin:0 auto 20px}
.icon svg{width:28px;height:28px;fill:#fff}
h1{font-size:20px;margin-bottom:8px;color:#16a34a}
p{font-size:14px;color:#666;line-height:1.6}
.detail{margin-top:20px;padding:12px;background:#f9fafb;border-radius:8px;font-size:13px;color:#888}
</style>
</head>
<body>
<div class="card">
<div class="icon"><svg viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg></div>
<h1>确认成功</h1>
<p>你已确认「%s」提醒，不再重复发送。</p>
<div class="detail">确认时间：%s</div>
</div>
</body>
</html>`, escapeHTML(title), confirmedAt)
}

// confirmAlreadyPage 重复点击页：把「已被使用」渲染成中性成功态并显示首次确认时间。
func confirmAlreadyPage(dl *models.DeliveryLog, loc *time.Location) string {
	title := "该提醒"
	if dl.Title != "" {
		title = dl.Title
	}
	confirmedAt := "此前"
	if dl.ConfirmedAt != nil {
		confirmedAt = formatInZone(*dl.ConfirmedAt, loc)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>已确认</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;background:#f5f5f5;color:#333}
.card{background:#fff;border-radius:12px;padding:40px;text-align:center;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:420px;margin:20px}
.icon{width:56px;height:56px;background:#22c55e;border-radius:50%%;display:flex;align-items:center;justify-content:center;margin:0 auto 20px}
.icon svg{width:28px;height:28px;fill:#fff}
h1{font-size:20px;margin-bottom:8px;color:#16a34a}
p{font-size:14px;color:#666;line-height:1.6}
.detail{margin-top:20px;padding:12px;background:#f9fafb;border-radius:8px;font-size:13px;color:#888}
</style>
</head>
<body>
<div class="card">
<div class="icon"><svg viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg></div>
<h1>之前已确认</h1>
<p>「%s」提醒已确认过了，不会重复发送。</p>
<div class="detail">确认时间：%s</div>
</div>
</body>
</html>`, escapeHTML(title), confirmedAt)
}

// confirmErrorPage 确认失败页
func confirmErrorPage(msg string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>确认失败</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;background:#f5f5f5;color:#333}
.card{background:#fff;border-radius:12px;padding:40px;text-align:center;box-shadow:0 2px 12px rgba(0,0,0,.08);max-width:420px;margin:20px}
.icon{width:56px;height:56px;background:#ef4444;border-radius:50%%;display:flex;align-items:center;justify-content:center;margin:0 auto 20px}
.icon svg{width:28px;height:28px;fill:#fff}
h1{font-size:20px;margin-bottom:8px;color:#dc2626}
p{font-size:14px;color:#666;line-height:1.6}
</style>
</head>
<body>
<div class="card">
<div class="icon"><svg viewBox="0 0 24 24"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg></div>
<h1>确认失败</h1>
<p>%s</p>
</div>
</body>
</html>`, escapeHTML(msg))
}

func escapeHTML(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			out = append(out, "&amp;"...)
		case '<':
			out = append(out, "&lt;"...)
		case '>':
			out = append(out, "&gt;"...)
		case '"':
			out = append(out, "&quot;"...)
		case '\'':
			out = append(out, "&#39;"...)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
