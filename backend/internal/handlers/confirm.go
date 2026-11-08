// confirm 处理确认链接：
// GET /c/:token 验证令牌并展示提醒详情及确认提示页，不直接消费令牌；
// POST /c/:token 消费令牌标记已确认，并终止重发链。
package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
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
// 仅展示待确认页面或已确认状态，不直接消费令牌，避免预览/爬虫误触。
func (h *ConfirmHandler) Confirm(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmErrorPage("确认链接无效")))
		return
	}

	dl, err := h.Svc.InspectToken(token)
	if err != nil {
		if errors.Is(err, services.ErrConfirmTokenUsed) && dl != nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmAlreadyPage(dl, h.loc())))
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmErrorPage(err.Error())))
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmPromptPage(token, dl, h.loc())))
}

// Submit 处理 POST /c/:token。
//
// 用户在确认页点击确认后提交，消费令牌并标记该链上的记录为已确认。
func (h *ConfirmHandler) Submit(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		if isJSONRequest(c) {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_TOKEN", "message": "确认链接无效"}})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmErrorPage("确认链接无效")))
		return
	}

	dl, err := h.Svc.ConsumeToken(token)
	if err != nil {
		if errors.Is(err, services.ErrConfirmTokenUsed) && dl != nil {
			if isJSONRequest(c) {
				c.JSON(http.StatusOK, gin.H{"code": 200, "message": "already_confirmed", "data": dl})
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmAlreadyPage(dl, h.loc())))
			return
		}
		if isJSONRequest(c) {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "CONFIRM_FAILED", "message": err.Error()}})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmErrorPage(err.Error())))
		return
	}

	if isJSONRequest(c) {
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": dl})
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(confirmSuccessPage(dl, h.loc())))
}

func isJSONRequest(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "application/json")
}

// formatInZone 把归一化为 UTC 的时间按站点时区格式化成 "2006-01-02 15:04:05"。
func formatInZone(t models.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return t.Std().In(loc).Format("2006-01-02 15:04:05")
}

// confirmPageBase 渲染具有统一样式、深色模式与返回后台按钮的完整 HTML 页面。
func confirmPageBase(pageTitle, iconType, iconSVG, titleClass, heading, descHTML, metaHTML, actionsHTML string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
<title>%s</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{
  --bg:#f8fafc;
  --card-bg:#ffffff;
  --text-main:#0f172a;
  --text-muted:#64748b;
  --border:#e2e8f0;
  --primary:#2563eb;
  --primary-hover:#1d4ed8;
  --primary-text:#ffffff;
  --secondary-bg:#f1f5f9;
  --secondary-hover:#e2e8f0;
  --secondary-text:#334155;
  --success-color:#16a34a;
  --danger-color:#dc2626;
  --card-shadow:0 10px 25px -5px rgba(0,0,0,.08),0 8px 10px -6px rgba(0,0,0,.04);
}
@media (prefers-color-scheme:dark){
  :root{
    --bg:#0f172a;
    --card-bg:#1e293b;
    --text-main:#f8fafc;
    --text-muted:#94a3b8;
    --border:#334155;
    --primary:#3b82f6;
    --primary-hover:#2563eb;
    --primary-text:#ffffff;
    --secondary-bg:#334155;
    --secondary-hover:#475569;
    --secondary-text:#f1f5f9;
    --success-color:#22c55e;
    --danger-color:#ef4444;
    --card-shadow:0 10px 25px -5px rgba(0,0,0,.35);
  }
}
html{height:100%%}
body{
  font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;
  display:flex;
  align-items:center;
  justify-content:center;
  min-height:100vh;
  min-height:100dvh;
  background:var(--bg);
  color:var(--text-main);
  padding:16px;
  box-sizing:border-box;
  overflow-x:hidden;
}
.card{
  background:var(--card-bg);
  border:1px solid var(--border);
  border-radius:16px;
  padding:28px 24px;
  text-align:center;
  box-shadow:var(--card-shadow);
  width:100%%;
  max-width:380px;
  box-sizing:border-box;
}
.icon{
  width:48px;
  height:48px;
  border-radius:50%%;
  display:flex;
  align-items:center;
  justify-content:center;
  margin:0 auto 16px;
}
.icon svg{width:24px;height:24px}
.icon.prompt{background:rgba(37,99,235,0.12);color:var(--primary)}
.icon.prompt svg{fill:currentColor}
.icon.success{background:rgba(22,163,74,0.12);color:var(--success-color)}
.icon.success svg{fill:currentColor}
.icon.danger{background:rgba(220,38,38,0.12);color:var(--danger-color)}
.icon.danger svg{fill:currentColor}
h1{font-size:18px;font-weight:600;margin-bottom:8px}
h1.prompt{color:var(--primary)}
h1.success{color:var(--success-color)}
h1.danger{color:var(--danger-color)}
p.desc{font-size:14px;color:var(--text-muted);line-height:1.5;margin-bottom:16px;word-break:break-word}
p.meta{font-size:12px;color:var(--text-muted);margin-bottom:16px}
.actions{display:flex;flex-direction:column;gap:8px}
.btn{
  display:inline-flex;
  align-items:center;
  justify-content:center;
  width:100%%;
  padding:10px 16px;
  border-radius:8px;
  font-size:14px;
  font-weight:500;
  text-decoration:none;
  cursor:pointer;
  border:1px solid transparent;
  transition:all .15s ease-in-out;
  box-sizing:border-box;
}
.btn-primary{background:var(--primary);color:var(--primary-text)}
.btn-primary:hover{background:var(--primary-hover)}
.btn-secondary{background:var(--secondary-bg);color:var(--secondary-text);border-color:var(--border)}
.btn-secondary:hover{background:var(--secondary-hover)}
.btn:disabled{opacity:0.6;cursor:not-allowed}
</style>
</head>
<body>
<div class="card">
<div class="icon %s">%s</div>
<h1 class="%s">%s</h1>
%s
%s
<div class="actions">
%s
</div>
</div>
</body>
</html>`,
		escapeHTML(pageTitle),
		iconType,
		iconSVG,
		titleClass,
		escapeHTML(heading),
		descHTML,
		metaHTML,
		actionsHTML,
	)
}

const (
	iconSVGBell  = `<svg viewBox="0 0 24 24"><path d="M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2zm6-6v-5c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2zm-2 1H8v-6c0-2.48 1.51-4.5 4-4.5s4 2.02 4 4.5v6z"/></svg>`
	iconSVGCheck = `<svg viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>`
	iconSVGCross = `<svg viewBox="0 0 24 24"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>`
)

// confirmPromptPage 待确认页面：提示用户点击确认，并提供返回后台链接。
func confirmPromptPage(token string, dl *models.DeliveryLog, loc *time.Location) string {
	title := "提醒事项"
	if dl != nil && dl.Title != "" {
		title = dl.Title
	}

	descHTML := fmt.Sprintf(`<p class="desc">请确认你已收到「%s」提醒，确认后将停止后续重发。</p>`, escapeHTML(title))
	metaHTML := ""

	actionsHTML := fmt.Sprintf(`
<form method="POST" action="/c/%s" onsubmit="var b=document.getElementById('confirm-btn');if(b){b.disabled=true;b.innerText='正在确认...'};">
  <button type="submit" id="confirm-btn" class="btn btn-primary">确认完成</button>
</form>
<a href="/" class="btn btn-secondary">返回后台</a>`, escapeHTML(token))

	return confirmPageBase("提醒确认", "prompt", iconSVGBell, "prompt", "提醒确认", descHTML, metaHTML, actionsHTML)
}

// confirmSuccessPage 确认成功页
func confirmSuccessPage(dl *models.DeliveryLog, loc *time.Location) string {
	title := "已确认"
	if dl != nil && dl.Title != "" {
		title = dl.Title
	}
	confirmedAt := formatInZone(models.TimeNow(), loc)
	if dl != nil && dl.ConfirmedAt != nil {
		confirmedAt = formatInZone(*dl.ConfirmedAt, loc)
	}

	descHTML := fmt.Sprintf(`<p class="desc">你已确认「%s」提醒，不再重复发送。</p>`, escapeHTML(title))
	metaHTML := fmt.Sprintf(`<p class="meta">确认时间：%s</p>`, confirmedAt)
	actionsHTML := `<a href="/" class="btn btn-primary">返回后台</a>`

	return confirmPageBase("确认成功", "success", iconSVGCheck, "success", "确认成功", descHTML, metaHTML, actionsHTML)
}

// confirmAlreadyPage 重复点击页：显示此前已确认及首次确认时间。
func confirmAlreadyPage(dl *models.DeliveryLog, loc *time.Location) string {
	title := "该提醒"
	if dl != nil && dl.Title != "" {
		title = dl.Title
	}
	confirmedAt := "此前"
	if dl != nil && dl.ConfirmedAt != nil {
		confirmedAt = formatInZone(*dl.ConfirmedAt, loc)
	}

	descHTML := fmt.Sprintf(`<p class="desc">「%s」提醒此前已确认过了，不会重复发送。</p>`, escapeHTML(title))
	metaHTML := fmt.Sprintf(`<p class="meta">确认时间：%s</p>`, confirmedAt)
	actionsHTML := `<a href="/" class="btn btn-primary">返回后台</a>`

	return confirmPageBase("已确认", "success", iconSVGCheck, "success", "之前已确认", descHTML, metaHTML, actionsHTML)
}

// confirmErrorPage 确认失败页
func confirmErrorPage(msg string) string {
	descHTML := fmt.Sprintf(`<p class="desc">%s</p>`, escapeHTML(msg))
	metaHTML := ""
	actionsHTML := `<a href="/" class="btn btn-secondary">返回后台</a>`

	return confirmPageBase("确认失败", "danger", iconSVGCross, "danger", "确认失败", descHTML, metaHTML, actionsHTML)
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
