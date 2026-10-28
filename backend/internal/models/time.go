package models

import (
	"database/sql/driver"
	"encoding/json"
	"strconv"
	"time"
)

// Time 是落库时间类型：在 SQLite 中以 Unix 毫秒（int64）存储。
//
// 为什么不直接用 time.Time：go-sqlite3 把 time.Time 写成
//
//	2006-01-02 15:04:05.999999999-07:00
//
// 这种变宽小数位、带时区偏移的文本，而 SQLite 对该列按字典序比较。
// 一旦两行的偏移不同（站点时区可在设置页热更新，历史行就是 +08:00），
// 同一时刻会被排到不同位置，范围查询与排序全部失真；唯一的补救是在 SQL 里
// 套 julianday()，但函数包裹列后果是**索引失效**（退化为全表扫描），
// 而且 julianday 是浮点天数，等值比较只到约 50µs 精度。
//
// 改存整数毫秒后：比较与排序都是整数语义，索引正常生效，等值精确，
// 也不再需要"记得把时间转成 UTC"这条纪律——Value 里已强制归一化。
//
// 内嵌 time.Time 保留 In / Format / Before / Equal / UTC 等方法，
// 上层代码几乎无需改动。零值表示"无此时间"，Value 会写成 NULL。
type Time struct {
	time.Time
}

// TimeOf 把 time.Time 转成 Time：归一化为 UTC 并截断到毫秒。
//
// 截断是为了让"内存里的值"与"库里读回来的值"完全一致——存储精度是毫秒，
// 若内存保留纳秒，往返比较（Equal）会莫名其妙地不相等。
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

// Scan 实现 sql.Scanner：主要吃整数毫秒，同时容忍历史遗留的文本时间
// （迁移前的行若因故残留，读取不会直接报错）。
func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		t.Time = time.Time{}
		return nil
	case int64:
		t.Time = time.UnixMilli(v).UTC()
		return nil
	case []byte:
		return t.scanText(string(v))
	case string:
		return t.scanText(v)
	case time.Time:
		t.Time = v.UTC()
		return nil
	default:
		t.Time = time.Time{}
		return nil
	}
}

// scanText 解析文本形态的时间：优先当毫秒整数，其次按常见时间格式解析。
func (t *Time) scanText(s string) error {
	if s == "" {
		t.Time = time.Time{}
		return nil
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		t.Time = time.UnixMilli(ms).UTC()
		return nil
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999-07:00",
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	// 无法识别的文本按"无时间"处理，避免一条脏数据让整个查询失败。
	t.Time = time.Time{}
	return nil
}

// GormDataType 让 GORM 把该类型映射成 integer 列（默认会用 string）。
func (Time) GormDataType() string {
	return "int"
}

// MarshalJSON 由内嵌的 time.Time 提供（输出 RFC3339 字符串），
// 对外接口契约因此与改造前完全一致。created_at / updated_at 这类自动字段
// 也走同一个 Value，落库精度同样是毫秒。
//
// UnmarshalJSON 解析 RFC3339 时间串（含 null）。
//
// 内嵌 time.Time 的 MarshalJSON 会被提升，但反序列化不会：json 会把 Time
// 当成普通 struct 去匹配字段，导致解码失败。显式实现一份，避免以后有人
// 把 models.Time 放进入参结构体时踩坑。
func (t *Time) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		t.Time = time.Time{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return t.scanText(s)
}
