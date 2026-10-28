// migrations 负责把旧版本以"带偏移的文本"落库的时间列，转成 models.Time
// 使用的整数毫秒形态。
//
// 为什么需要：改造前时间字段是 time.Time，go-sqlite3 会写成
// "2026-06-26 12:00:00.123456789+08:00" 这样的文本。SQLite 对这类列按字典序
// 比较，偏移不同的两行会被排错位置；查询只能靠 julianday() 包裹，代价是索引失效。
// 现在统一存整数毫秒，比较与排序都是整数语义，索引也能正常命中。
//
// 升级时该列会被 AutoMigrate 重建成 INTEGER，但值仍是原来的文本（SQLite 的
// 类型亲和性不会把 "2026-06-26 12:00:00+00:00" 转成数字），所以还要按值做一次
// 显式转换，否则新旧数据会在同一列里混着两种形态，范围查询又只命中一半。
package database

import (
	"fmt"
	"log/slog"
	"reflect"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// timeReflectTypes 是需要归一化的字段类型集合（值与指针两种形态）。
var timeReflectTypes = []reflect.Type{
	reflect.TypeOf(models.Time{}),
	reflect.TypeOf(&models.Time{}),
}

// allModels 是 AutoMigrate 使用的模型清单，也用于自动发现时间列。
//
// 与 Init 里传给 AutoMigrate 的列表必须保持一致——两处共用这一个变量即可，
// 避免新增模型后迁移时间列时漏掉。
var allModels = []any{
	&models.User{},
	&models.Reminder{},
	&models.ReminderChannel{},
	&models.Channel{},
	&models.DeliveryLog{},
	&models.DeliveryAttempt{},
	&models.Token{},
	&models.TokenDefaultChannel{},
	&models.ConfirmToken{},
	&models.Config{},
}

// timeColumn 描述一个需要归一化的时间列。
type timeColumn struct {
	Table  string
	Column string
}

// discoverTimeColumns 从 GORM schema 里自动找出所有 models.Time 类型的列。
//
// 不写死表名/列名清单：那样每加一个时间字段就得记得回来登记，
// 漏一次就会让旧库升级后该列还留着文本，而这个错误没有任何报错，只有查询悄悄少结果。
func discoverTimeColumns(db *gorm.DB) ([]timeColumn, error) {
	var out []timeColumn
	for _, m := range allModels {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(m); err != nil {
			return nil, fmt.Errorf("解析模型 %T 失败: %w", m, err)
		}
		for _, field := range stmt.Schema.Fields {
			for _, want := range timeReflectTypes {
				if field.FieldType == want {
					out = append(out, timeColumn{Table: stmt.Schema.Table, Column: field.DBName})
					break
				}
			}
		}
	}
	return out, nil
}

// normalizeTimeColumns 把仍是文本形态的时间值就地转换成 Unix 毫秒。
//
// 幂等：已经是整数的行不会被再次处理。返回实际转换的总行数。
//
// 无法被 SQLite 解析的脏值（如空串、非法文本）统一转成 NULL——保留原文本会
// 让整数列里混入字符串，排序和比较结果不可预期。
func normalizeTimeColumns(db *gorm.DB) (int64, error) {
	columns, err := discoverTimeColumns(db)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, tc := range columns {
		if !db.Migrator().HasTable(tc.Table) || !db.Migrator().HasColumn(tc.Table, tc.Column) {
			continue
		}

		// typeof(col) = 'text' 才需要转换；NULL 与 integer 都跳过。
		// julianday() 能解析 go-sqlite3 写出的全部文本形态（含纳秒与偏移），
		// 2440587.5 是 Unix 纪元对应的儒略日；(jd - 2440587.5) * 86400000 得到毫秒。
		// 不用 strftime('%s') 是因为它只到秒，会丢掉小数部分。
		// 解析失败时 julianday 返回 NULL，整个表达式也就写入 NULL，正好对应"无此时间"。
		stmt := fmt.Sprintf(
			"UPDATE %s SET %s = CAST(ROUND((julianday(%s) - 2440587.5) * 86400000) AS INTEGER) WHERE typeof(%s) = 'text'",
			tc.Table, tc.Column, tc.Column, tc.Column,
		)
		res := db.Exec(stmt)
		if res.Error != nil {
			return total, fmt.Errorf("转换 %s.%s 失败: %w", tc.Table, tc.Column, res.Error)
		}
		if res.RowsAffected > 0 {
			slog.Info("时间列已转换为整数毫秒",
				"table", tc.Table, "column", tc.Column, "rows", res.RowsAffected)
			total += res.RowsAffected
		}
	}
	return total, nil
}
