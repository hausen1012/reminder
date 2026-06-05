package scheduler

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	return loc
}

func TestComputeSolarOnce_Future(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{At: "2026-06-10T09:00:00"}
	after := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := Compute("solar", "once", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil time")
	}
	want := time.Date(2026, 6, 10, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSolarOnce_Past(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{At: "2020-01-01T00:00:00"}
	after := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := Compute("solar", "once", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("过去时间应返回 nil，得到 %s", got)
	}
}

func TestComputeSolarInterval_Minute(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{
		StartAt: "2026-06-05T09:00:00",
		Every:   10,
		Unit:    "minute",
	}
	after := time.Date(2026, 6, 5, 9, 25, 0, 0, loc)
	got, err := Compute("solar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 6, 5, 9, 30, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSolarInterval_Day(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{
		StartAt: "2026-06-05T08:00:00",
		Every:   1,
		Unit:    "day",
	}
	after := time.Date(2026, 6, 6, 7, 0, 0, 0, loc)
	got, err := Compute("solar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 6, 6, 8, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSolarInterval_MonthRollover(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{
		StartAt: "2026-01-31T09:00:00",
		Every:   1,
		Unit:    "month",
	}
	// 从 1/31 → 2/28（time.AddDate 自动归一），再 → 3/28
	after := time.Date(2026, 2, 28, 10, 0, 0, 0, loc)
	got, err := Compute("solar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Month() != time.March {
		t.Errorf("月份应是 3 月，得到 %s", got)
	}
}

func TestComputeSolarInterval_FirstFireIsStartAt(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{
		StartAt: "2026-06-10T09:00:00",
		Every:   1,
		Unit:    "day",
	}
	// after 早于 start_at：下一次就是 start_at 本身
	after := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := Compute("solar", "interval", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := time.Date(2026, 6, 10, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSolarCron(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{Expr: "0 9 * * 1-5"} // 工作日早 9 点
	after := time.Date(2026, 6, 5, 10, 0, 0, 0, loc) // 周五 10:00
	got, err := Compute("solar", "cron", spec, after, loc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// 下一次是下周一 09:00
	if got.Weekday() != time.Monday || got.Hour() != 9 {
		t.Errorf("期望下周一 09:00，得到 %s", got)
	}
}

func TestComputeSolarCron_InvalidExpr(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{Expr: "not a cron"}
	if _, err := Compute("solar", "cron", spec, time.Now(), loc); err == nil {
		t.Fatal("无效 cron 应报错")
	}
}

func TestParseSpec_RoundTrip(t *testing.T) {
	raw := []byte(`{"at":"2026-06-10T09:00:00"}`)
	s, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if s.At != "2026-06-10T09:00:00" {
		t.Errorf("got %q", s.At)
	}
}

func TestCompute_LunarReturnsError(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	_, err := Compute("lunar", "once", &ScheduleSpec{}, time.Now(), loc)
	if err == nil {
		t.Fatal("Phase 2 应返回 lunar 暂未实现错误")
	}
}

func TestPreviewSolar_Cron(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{Expr: "0 9 * * *"}
	from := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := PreviewSolar("solar", "cron", spec, from, loc, 3)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	for _, t0 := range got {
		if t0.Hour() != 9 {
			t.Errorf("每次应在 09 点，得到 %s", t0)
		}
	}
}

func TestPreviewSolar_OnceStopsEarly(t *testing.T) {
	loc := mustLoc(t, "Asia/Shanghai")
	spec := &ScheduleSpec{At: "2026-06-10T09:00:00"}
	from := time.Date(2026, 6, 5, 8, 0, 0, 0, loc)
	got, err := PreviewSolar("solar", "once", spec, from, loc, 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("once 只应返回 1 次，得到 %d", len(got))
	}
}
