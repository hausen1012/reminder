// Package scheduler 提供触发计算、定时调度与补漏扫描组件。
package scheduler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/6tail/lunar-go/calendar"
	"github.com/robfig/cron/v3"
)

// ScheduleSpec 是 Reminder.ScheduleSpec 反序列化后的结构。
//
// 一个 spec 只会用到对应 (Calendar, ScheduleType) 的子集字段，其他字段为零值。
type ScheduleSpec struct {
	// solar/once
	At string `json:"at,omitempty"` // 配置时区下的本地时间 "2006-01-02T15:04:05"

	// solar/interval
	StartAt string `json:"start_at,omitempty"`
	Every   int    `json:"every,omitempty"`
	Unit    string `json:"unit,omitempty"` // minute | hour | day | month | year

	// solar/cron
	Expr string `json:"expr,omitempty"`

	// lunar/once
	Lunar  *LunarDate `json:"lunar,omitempty"`
	Hour   int        `json:"hour,omitempty"`
	Minute int        `json:"minute,omitempty"`

	// lunar/interval
	StartLunar *LunarDate `json:"start_lunar,omitempty"`
	LeapPolicy string     `json:"leap_policy,omitempty"` // 仅 "skip"
	SizePolicy string     `json:"size_policy,omitempty"` // 仅 "shift"
}

// LunarDate 表示一个农历日期（公历转换需要外部库辅助）。
type LunarDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// ParseSpec 反序列化 ScheduleSpec JSON。
func ParseSpec(raw []byte) (*ScheduleSpec, error) {
	var s ScheduleSpec
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("解析 schedule_spec 失败: %w", err)
	}
	return &s, nil
}

// Compute 根据 (calendar, scheduleType, spec, after, loc) 算出下一次触发时间。
//
// 返回值的时区与 loc 一致；返回 nil 表示该 spec 已不会再触发（如单次已过期）。
func Compute(calendar, scheduleType string, spec *ScheduleSpec, after time.Time, loc *time.Location) (*time.Time, error) {
	if spec == nil {
		return nil, fmt.Errorf("spec 不能为空")
	}
	if loc == nil {
		loc = time.Local
	}
	switch calendar {
	case "solar":
		switch scheduleType {
		case "once":
			return computeSolarOnce(spec, after, loc)
		case "interval":
			return computeSolarInterval(spec, after, loc)
		case "cron":
			return computeSolarCron(spec, after, loc)
		}
	case "lunar":
		switch scheduleType {
		case "once":
			return computeLunarOnce(spec, after, loc)
		case "interval":
			return computeLunarInterval(spec, after, loc)
		}
	}
	return nil, fmt.Errorf("不支持的调度组合: %s/%s", calendar, scheduleType)
}

func computeSolarOnce(s *ScheduleSpec, after time.Time, loc *time.Location) (*time.Time, error) {
	if s.At == "" {
		return nil, fmt.Errorf("solar/once 需要字段 at")
	}
	t, err := parseLocalTime(s.At, loc)
	if err != nil {
		return nil, fmt.Errorf("解析 at 失败: %w", err)
	}
	if !t.After(after) {
		return nil, nil
	}
	return &t, nil
}

func computeSolarInterval(s *ScheduleSpec, after time.Time, loc *time.Location) (*time.Time, error) {
	if s.StartAt == "" || s.Every <= 0 || s.Unit == "" {
		return nil, fmt.Errorf("solar/interval 需要 start_at + every + unit")
	}
	start, err := parseLocalTime(s.StartAt, loc)
	if err != nil {
		return nil, fmt.Errorf("解析 start_at 失败: %w", err)
	}
	// 第一次触发就是 start_at；之后按 every*unit 累加，month/year 用 AddDate 保留日历语义
	t := start
	// 保险上限，避免异常参数导致死循环
	const maxIters = 1 << 20
	for i := 0; i < maxIters; i++ {
		if t.After(after) {
			return &t, nil
		}
		next, err := addInterval(t, s.Every, s.Unit)
		if err != nil {
			return nil, err
		}
		if !next.After(t) {
			return nil, fmt.Errorf("interval 计算未推进：every=%d unit=%s", s.Every, s.Unit)
		}
		t = next
	}
	return nil, fmt.Errorf("interval 循环超过上限")
}

func computeSolarCron(s *ScheduleSpec, after time.Time, loc *time.Location) (*time.Time, error) {
	if s.Expr == "" {
		return nil, fmt.Errorf("solar/cron 需要字段 expr")
	}
	sched, err := cron.ParseStandard(s.Expr)
	if err != nil {
		return nil, fmt.Errorf("解析 cron 表达式失败: %w", err)
	}
	// 把 after 转到 loc 时区，再让 cron 计算下一次
	t := sched.Next(after.In(loc))
	if t.IsZero() {
		return nil, nil
	}
	return &t, nil
}

// addInterval 按 unit 累加 every 个单位；day/month/year 用 AddDate 保留日历语义。
func addInterval(t time.Time, every int, unit string) (time.Time, error) {
	switch unit {
	case "minute":
		return t.Add(time.Duration(every) * time.Minute), nil
	case "hour":
		return t.Add(time.Duration(every) * time.Hour), nil
	case "day":
		return t.AddDate(0, 0, every), nil
	case "month":
		return t.AddDate(0, every, 0), nil
	case "year":
		return t.AddDate(every, 0, 0), nil
	}
	return time.Time{}, fmt.Errorf("不支持的 unit: %s", unit)
}

// --- 农历计算 ---

func computeLunarOnce(s *ScheduleSpec, after time.Time, loc *time.Location) (*time.Time, error) {
	if s.Lunar == nil {
		return nil, fmt.Errorf("lunar/once 需要字段 lunar")
	}
	lun := calendar.NewLunarFromYmd(s.Lunar.Year, s.Lunar.Month, s.Lunar.Day)
	sol := lun.GetSolar()
	t := time.Date(sol.GetYear(), time.Month(sol.GetMonth()), sol.GetDay(),
		s.Hour, s.Minute, 0, 0, loc)
	if !t.After(after) {
		return nil, nil
	}
	return &t, nil
}

func computeLunarInterval(s *ScheduleSpec, after time.Time, loc *time.Location) (*time.Time, error) {
	if s.StartLunar == nil || s.Every <= 0 || s.Unit == "" {
		return nil, fmt.Errorf("lunar/interval 需要 start_lunar + every + unit")
	}
	lun := calendar.NewLunarFromYmd(s.StartLunar.Year, s.StartLunar.Month, s.StartLunar.Day)
	sol := lun.GetSolar()
	start := time.Date(sol.GetYear(), time.Month(sol.GetMonth()), sol.GetDay(),
		s.Hour, s.Minute, 0, 0, loc)

	t := start
	const maxIters = 1 << 20
	for i := 0; i < maxIters; i++ {
		if t.After(after) {
			return &t, nil
		}
		next, err := addLunarInterval(t, s.Every, s.Unit, loc)
		if err != nil {
			return nil, err
		}
		if !next.After(t) {
			return nil, fmt.Errorf("lunar interval 计算未推进：every=%d unit=%s", s.Every, s.Unit)
		}
		t = next
	}
	return nil, fmt.Errorf("lunar interval 循环超过上限")
}

// addLunarInterval 按 unit 在农历上累加：day 用 Next(days)，month/year 对农历月/年做加法
// （size_policy=shift 处理月末，leap_policy=skip 只产生非闰月日期）。
func addLunarInterval(t time.Time, every int, unit string, loc *time.Location) (time.Time, error) {
	switch unit {
	case "day":
		lun := calendar.NewLunarFromSolar(calendar.NewSolarFromYmd(t.Year(), int(t.Month()), t.Day()))
		nextLun := lun.Next(every)
		sol := nextLun.GetSolar()
		return time.Date(sol.GetYear(), time.Month(sol.GetMonth()), sol.GetDay(),
			t.Hour(), t.Minute(), 0, 0, loc), nil
	case "month":
		return addLunarMonths(t, every, loc)
	case "year":
		return addLunarYears(t, every, loc)
	}
	return time.Time{}, fmt.Errorf("lunar 不支持的 unit: %s", unit)
}

// addLunarMonths 在农历月份上累加：只对应非闰月，超 12 进位到年，月末顺延。
func addLunarMonths(t time.Time, months int, loc *time.Location) (time.Time, error) {
	lun := calendar.NewLunarFromSolar(calendar.NewSolarFromYmd(t.Year(), int(t.Month()), t.Day()))
	ly := lun.GetYear()
	lm := lun.GetMonth()
	if lm < 0 {
		lm = -lm // 若落在闰月，取其对应的非闰月编号
	}
	ld := lun.GetDay()

	lm += months
	for lm > 12 {
		ly++
		lm -= 12
	}
	for lm < 1 {
		ly--
		lm += 12
	}

	targetMonth := calendar.NewLunarMonthFromYm(ly, lm)
	if ld > targetMonth.GetDayCount() {
		ld = targetMonth.GetDayCount() // shift
	}

	nextLun := calendar.NewLunarFromYmd(ly, lm, ld)
	sol := nextLun.GetSolar()
	return time.Date(sol.GetYear(), time.Month(sol.GetMonth()), sol.GetDay(),
		t.Hour(), t.Minute(), 0, 0, loc), nil
}

// addLunarYears 在农历年份上累加，月日不变。
func addLunarYears(t time.Time, years int, loc *time.Location) (time.Time, error) {
	lun := calendar.NewLunarFromSolar(calendar.NewSolarFromYmd(t.Year(), int(t.Month()), t.Day()))
	ly := lun.GetYear() + years
	lm := lun.GetMonth()
	if lm < 0 {
		lm = -lm
	}
	ld := lun.GetDay()

	targetMonth := calendar.NewLunarMonthFromYm(ly, lm)
	if ld > targetMonth.GetDayCount() {
		ld = targetMonth.GetDayCount() // shift
	}

	nextLun := calendar.NewLunarFromYmd(ly, lm, ld)
	sol := nextLun.GetSolar()
	return time.Date(sol.GetYear(), time.Month(sol.GetMonth()), sol.GetDay(),
		t.Hour(), t.Minute(), 0, 0, loc), nil
}

// parseLocalTime 解析 "YYYY-MM-DDTHH:MM[:SS]" 为指定时区时间，允许末尾不带秒。
func parseLocalTime(s string, loc *time.Location) (time.Time, error) {
	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("时间格式不识别: %s", s)
}

// PreviewSolar 返回未来 n 次触发时间，用于编辑页的「下次触发预览」。
func PreviewSolar(calendar, scheduleType string, spec *ScheduleSpec, from time.Time, loc *time.Location, n int) ([]time.Time, error) {
	out := make([]time.Time, 0, n)
	cur := from
	for i := 0; i < n; i++ {
		nxt, err := Compute(calendar, scheduleType, spec, cur, loc)
		if err != nil {
			return nil, err
		}
		if nxt == nil {
			break
		}
		out = append(out, *nxt)
		cur = *nxt
	}
	return out, nil
}
