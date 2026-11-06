package tenancy

import (
	"reflect"
	"strings"

	"gorm.io/gorm"
)

// Tx 是租户隔离下的数据访问句柄。业务只能通过它访问数据库。
type Tx interface {
	// 写：tenant_id 由 Callback 填充，业务不写。
	Create(dst any) error
	Update(dst any, values map[string]any) error
	Delete(dst any) error
	// DeleteAll 删除当前租户下的全部行，仅用于租户数据清理。
	DeleteAll(dst any) error

	// 链式条件：返回同一事务内的新句柄，不改动原句柄。
	Model(dst any) Tx
	Where(query any, args ...any) Tx
	Order(value any) Tx
	Limit(n int) Tx
	Offset(n int) Tx
	Select(query any, args ...any) Tx

	// 终止操作。
	Find(dst any) error
	First(dst any) error
	Count(count *int64) error
	Pluck(column string, dst any) error
	Scan(dst any) error

	// Raw 只用于 ORM 表达不了的聚合 / 报表查询：
	// 单条参数化 SELECT，tenant_id 条件由调用方显式写出（readme 3.2.1）。
	Raw(sql string, args ...any) Tx
}

type rawQuery struct {
	sql  string
	args []any
}

type tx struct {
	gorm *gorm.DB
	raw  *rawQuery
	cond bool // 是否已经挂过 Where
}

func (t *tx) session() *gorm.DB {
	return t.gorm.Session(&gorm.Session{})
}

// Create 支持单条与切片批量；tenant_id 由 Callback 统一填充。
func (t *tx) Create(dst any) error {
	if t.raw != nil {
		return ErrRawWrite
	}
	return t.gorm.Create(dst).Error
}

// Update 按主键或已挂 Where 条件更新指定列。
func (t *tx) Update(dst any, values map[string]any) error {
	if t.raw != nil {
		return ErrRawWrite
	}
	if len(values) == 0 {
		return ErrEmptyValues
	}
	if !t.cond && !hasPrimaryKeyValue(t.gorm, dst) {
		return ErrMissingCondition
	}

	return t.gorm.Model(dst).Updates(values).Error
}

// Delete 按主键或已挂 Where 条件删除。
func (t *tx) Delete(dst any) error {
	if t.raw != nil {
		return ErrRawWrite
	}
	if !t.cond && !hasPrimaryKeyValue(t.gorm, dst) {
		return ErrMissingCondition
	}

	return t.gorm.Delete(dst).Error
}

// DeleteAll 按当前 Scope 删除整表范围内的数据，tenant_id 由 Callback 限制。
func (t *tx) DeleteAll(dst any) error {
	if t.raw != nil {
		return ErrRawWrite
	}

	return t.gorm.Where("1 = 1").Delete(dst).Error
}

func (t *tx) Model(dst any) Tx {
	return &tx{gorm: t.session().Model(dst), cond: t.cond}
}

func (t *tx) Where(query any, args ...any) Tx {
	return &tx{gorm: t.session().Where(query, args...), cond: true}
}

func (t *tx) Order(value any) Tx {
	return &tx{gorm: t.session().Order(value), cond: t.cond}
}

func (t *tx) Limit(n int) Tx {
	return &tx{gorm: t.session().Limit(n), cond: t.cond}
}

func (t *tx) Offset(n int) Tx {
	return &tx{gorm: t.session().Offset(n), cond: t.cond}
}

func (t *tx) Select(query any, args ...any) Tx {
	return &tx{gorm: t.session().Select(query, args...), cond: t.cond}
}

func (t *tx) Find(dst any) error {
	if t.raw != nil {
		return ErrRawTerminal
	}
	return t.gorm.Find(dst).Error
}

func (t *tx) First(dst any) error {
	if t.raw != nil {
		return ErrRawTerminal
	}
	return t.gorm.First(dst).Error
}

func (t *tx) Count(count *int64) error {
	if t.raw != nil {
		wrapped := "SELECT COUNT(*) FROM (" + trimSQL(t.raw.sql) + ") AS tenancy_raw_count"
		return t.gorm.Raw(wrapped, t.raw.args...).Scan(count).Error
	}

	return t.gorm.Count(count).Error
}

func (t *tx) Pluck(column string, dst any) error {
	if t.raw != nil {
		return ErrRawTerminal
	}
	return t.gorm.Pluck(column, dst).Error
}

// Scan 把 Raw 结果映射到 dst（结构体、结构体切片或 map）。
func (t *tx) Scan(dst any) error {
	if t.raw != nil {
		return t.gorm.Raw(t.raw.sql, t.raw.args...).Scan(dst).Error
	}

	return t.gorm.Scan(dst).Error
}

// Raw 只接受单条参数化 SQL，不做任何改写（规范见 readme 3.2.1）。
func (t *tx) Raw(sql string, args ...any) Tx {
	return &tx{gorm: t.gorm, raw: &rawQuery{sql: sql, args: args}, cond: t.cond}
}

func trimSQL(sql string) string {
	return strings.TrimSuffix(strings.TrimSpace(sql), ";")
}

// hasPrimaryKeyValue 判断 dst 是否带了可用的主键值，避免无条件的全表更新 / 删除。
func hasPrimaryKeyValue(db *gorm.DB, dst any) bool {
	value := reflect.Indirect(reflect.ValueOf(dst))
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return false
	}

	stmt := db.Session(&gorm.Session{NewDB: true})
	stmt.Statement.Dest = dst
	if err := stmt.Statement.Parse(dst); err != nil {
		return false
	}

	schema := stmt.Statement.Schema
	if schema == nil || len(schema.PrimaryFields) == 0 {
		return false
	}

	ctx := stmt.Statement.Context
	for _, field := range schema.PrimaryFields {
		if _, isZero := field.ValueOf(ctx, value); isZero {
			return false
		}
	}

	return true
}
