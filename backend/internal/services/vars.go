// vars 构建一次触发的模板变量 map。
//
// 设计文档 §6.3 列出的内置变量：
//   - now / now_date / now_time
//   - fire_at  （触发计划时间，便于补漏场景与现在时间区分）
//   - title / content     提醒标题与原始内容
//   - lunar_date          当前公历日期对应的农历串
//   - reminder_id / reminder_source
//   - confirm_url         需要确认的提醒由 dispatch 注入
package services

import (
	"fmt"
	"time"

	"github.com/6tail/lunar-go/calendar"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/notifier"
)

var weekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// buildVars 构造一次触发渲染用的变量 map。
//
// firedAt 是真实开始执行 dispatch 的时间；planned 是计划触发时间。
// loc 是配置时区，用于格式化展示。
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
		"trigger_at":      fireAt.Format("2006-01-02 15:04:05"),
		"trigger_date":    fireAt.Format("2006-01-02"),
		"trigger_time":    fireAt.Format("15:04"),
		"weekday":         weekdays[fireAt.Weekday()],
		"title":           r.Title,
		"content":         r.Content,
		"lunar_date":      lunarStr,
		"reminder_id":     fmt.Sprintf("%d", r.ID),
		"reminder_source": r.Source,
		"source":          r.Source,
	}
	return vars
}

// buildRenderedMessage 是渲染提醒内容的唯一入口。
//
// 要点（也是常被遗漏的坑）：
//  1. 用 vars 渲染 body（展开 {{now_date}} {{lunar_date}}  占位符）
//  2. 用渲染后的 body 覆盖 vars["content"]，
//     保证下游各通道（尤其是 Webhook BodyTemplate）中的 {{content}} 拿到的是渲染结果而非原始模板
//  3. 渲染结束后 vars 是最终态，所有后续的二次渲染（如 Webhook 的 body_template 二次渲染）
//     都基于这套 vars
//
// 所有构造 notifier.Message 的调用方都应走此函数，禁止手拼 notifier.Message{}。
func buildRenderedMessage(title, content, contentFormat string, vars map[string]string) notifier.Message {
	body := notifier.Render(content, vars)
	vars["content"] = body
	return notifier.Message{
		Subject: notifier.Render(title, vars),
		Body:    body,
		Format:  contentFormat,
		Vars:    vars,
	}
}

// formatLunar 把公历时间转成"农历xxxx年x月初x"中文串，方便用户阅读。
func formatLunar(t time.Time) string {
	sol := calendar.NewSolarFromYmd(t.Year(), int(t.Month()), t.Day())
	lun := calendar.NewLunarFromSolar(sol)
	return fmt.Sprintf("农历%s年%s月%s",
		lun.GetYearInChinese(), lun.GetMonthInChinese(), lun.GetDayInChinese())
}