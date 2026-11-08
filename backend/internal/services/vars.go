// vars 构建一次触发的模板变量 map（now / fire_at / title / content / lunar_date /
// reminder_id / reminder_source 等，confirm_url 由 dispatch 注入）。
package services

import (
	"fmt"
	"time"

	"github.com/6tail/lunar-go/calendar"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/notifier"
)

var weekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// buildVars 构造一次触发的渲染变量；firedAt 是实际投递时间，planned 是计划触发时间。
func buildVars(r *models.Reminder, firedAt, planned time.Time, loc *time.Location) map[string]string {
	if loc == nil {
		loc = time.Local
	}
	now := firedAt.In(loc)
	fireAt := planned.In(loc)
	lunarStr := formatLunar(now)

	var vars = map[string]string{
		"now":             now.Format("2006-01-02 15:04:05"),
		"now_date":        now.Format("2006-01-02"),
		"now_time":        now.Format("15:04:05"),
		"fire_at":         fireAt.Format("2006-01-02 15:04:05"),
		"trigger_date":    fireAt.Format("2006-01-02"),
		"trigger_time":    fireAt.Format("15:04"),
		"weekday":         weekdays[fireAt.Weekday()],
		"title":           r.Title,
		"content":         r.Content,
		"lunar_date":      lunarStr,
		"reminder_id":     fmt.Sprintf("%d", r.ID),
		"reminder_source": r.Source,
	}
	return vars
}

// buildRenderedMessage 渲染标题与正文，并把展开后的结果写回 vars["title"] / vars["content"]，
// 保证下游（尤其 Webhook 模板）拿到的是展开后的内容。正文先渲染：标题里可以引用 {{content}}。
//
// 例外：DispatchService.Run（需先注入 confirm_url）与 ChannelService.DryRun（纯试发）
// 自行构造 Message，是有意保留的分叉。
func buildRenderedMessage(title, content, contentFormat string, vars map[string]string) notifier.Message {
	body := notifier.Render(content, vars)
	vars["content"] = body
	subject := notifier.Render(title, vars)
	vars["title"] = subject
	return notifier.Message{
		Subject: subject,
		Body:    body,
		Format:  contentFormat,
		Vars:    vars,
	}
}

// formatLunar 把公历时间转成「农历二〇二六年五月十七」形式的中文串。
func formatLunar(t time.Time) string {
	sol := calendar.NewSolarFromYmd(t.Year(), int(t.Month()), t.Day())
	lun := calendar.NewLunarFromSolar(sol)
	return fmt.Sprintf("农历%s年%s月%s",
		lun.GetYearInChinese(), lun.GetMonthInChinese(), lun.GetDayInChinese())
}
