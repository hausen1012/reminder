package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// Time 在 SQLite 中以 Unix 毫秒（int64）存储，零值写为 NULL。
type Time struct {
	time.Time
}

// TimeOf 把 time.Time 转成 Time：归一化为 UTC 并截断到毫秒。
func TimeOf(t time.Time) Time {
	if t.IsZero() {
		return Time{}
	}
	return Time{Time: t.UTC().Truncate(time.Millisecond)}
}

// TimeNow 返回当前时刻的 Time。
func TimeNow() Time {
	return TimeOf(time.Now())
}

// TimePtrOf 把 time.Time 转成 *Time，便于写入可空字段。
func TimePtrOf(t time.Time) *Time {
	v := TimeOf(t)
	return &v
}

// TimePtr 把可空 time.Time 转成可空 *Time；nil 原样返回（落库为 NULL）。
func TimePtr(t *time.Time) *Time {
	if t == nil {
		return nil
	}
	return TimePtrOf(*t)
}

// Std 返回底层的 time.Time。
func (t Time) Std() time.Time {
	return t.Time
}

// Value 实现 driver.Valuer：写成 Unix 毫秒整数，零值写 NULL。
func (t Time) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return t.Time.UTC().UnixMilli(), nil
}

// Scan 实现 sql.Scanner：读取 Unix 毫秒整数。
func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		t.Time = time.Time{}
		return nil
	case int64:
		t.Time = time.UnixMilli(v).UTC()
		return nil
	case time.Time:
		t.Time = v.UTC().Truncate(time.Millisecond)
		return nil
	default:
		t.Time = time.Time{}
		return nil
	}
}

// GormDataType 让 GORM 把该类型映射成 integer 列。
func (Time) GormDataType() string {
	return "int"
}

// MarshalJSON 由内嵌的 time.Time 提供（输出 RFC3339 字符串）。
// UnmarshalJSON 解析 RFC3339 时间串：null 与空串都表示"无此时间"。
func (t *Time) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		t.Time = time.Time{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		t.Time = time.Time{}
		return nil
	}
	// RFC3339Nano 的布局里小数位是可选的，一个布局就能覆盖带/不带毫秒两种写法
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	t.Time = parsed.UTC().Truncate(time.Millisecond)
	return nil
}
