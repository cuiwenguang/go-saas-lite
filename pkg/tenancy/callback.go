package tenancy

import (
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// TenantColumn 是所有租户组表共用的隔离列名。
const TenantColumn = "tenant_id"

// RegisterTenantCallbacks 把租户隔离 Callback 注册到指定连接。
// 只有租户连接池调用它；平台连接池不注册。
func RegisterTenantCallbacks(db *gorm.DB) {
	_ = db.Callback().Create().Before("gorm:create").Register("tenancy:set_tenant", setTenantID)
	_ = db.Callback().Query().Before("gorm:query").Register("tenancy:add_tenant_where", addTenantWhere)
	_ = db.Callback().Update().Before("gorm:update").Register("tenancy:add_tenant_where", addTenantWhere)
	_ = db.Callback().Delete().Before("gorm:delete").Register("tenancy:add_tenant_where", addTenantWhere)
}

func tenantField(db *gorm.DB) *schema.Field {
	if db.Statement.Schema == nil {
		return nil
	}
	return db.Statement.Schema.LookUpField(TenantColumn)
}

// setTenantID 在插入前用 Scope 覆盖 tenant_id，业务传入的值不生效。
func setTenantID(db *gorm.DB) {
	field := tenantField(db)
	if field == nil {
		db.AddError(ErrNotTenantTable)
		return
	}

	scope, err := ScopeFrom(db.Statement.Context)
	if err != nil {
		db.AddError(err)
		return
	}

	value := db.Statement.ReflectValue
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			elem := value.Index(i)
			if elem.Kind() == reflect.Ptr {
				if elem.IsNil() {
					continue
				}
				elem = elem.Elem()
			}
			if err := field.Set(db.Statement.Context, elem, scope.TenantID); err != nil {
				db.AddError(err)
				return
			}
		}
	default:
		if err := field.Set(db.Statement.Context, value, scope.TenantID); err != nil {
			db.AddError(err)
		}
	}
}

// addTenantWhere 在查询 / 更新 / 删除前追加 tenant_id 条件。
// 已有条件通过 AND 拼接，不覆盖业务条件。
func addTenantWhere(db *gorm.DB) {
	if db.Statement.Schema == nil {
		// Schema 未知（Raw、Table(...) 等原生路径）：按 readme 3.2.1 由调用方显式写租户条件。
		return
	}

	field := db.Statement.Schema.LookUpField(TenantColumn)
	if field == nil {
		// 有模型却没有 tenant_id：说明是平台组表，不许走租户连接。
		db.AddError(ErrNotTenantTable)
		return
	}

	scope, err := ScopeFrom(db.Statement.Context)
	if err != nil {
		db.AddError(err)
		return
	}

	db.Statement.AddClause(clause.Where{
		Exprs: []clause.Expression{
			clause.Eq{
				Column: clause.Column{Table: db.Statement.Table, Name: TenantColumn},
				Value:  scope.TenantID,
			},
		},
	})
}
