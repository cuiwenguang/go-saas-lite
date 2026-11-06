// Package authz 负责权限点定义与角色求值。
//
// 权限点 = 资源 + 动作，落库格式 obj:act（readme 2.3）。
// 业务只依赖 Authorizer，不直接读角色表。
package authz

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Perm 是一个权限点。
type Perm struct {
	Obj string
	Act string
}

// 标准动作固定四个；read 同时覆盖列表与详情。
func Read(obj string) Perm        { return Perm{Obj: obj, Act: "read"} }
func Create(obj string) Perm      { return Perm{Obj: obj, Act: "create"} }
func Update(obj string) Perm      { return Perm{Obj: obj, Act: "update"} }
func Delete(obj string) Perm      { return Perm{Obj: obj, Act: "delete"} }
func Custom(obj, act string) Perm { return Perm{Obj: obj, Act: act} }

// Wildcard 表示全部权限，admin 存 ["*:*"]。
func Wildcard() Perm { return Perm{Obj: "*", Act: "*"} }

// String 返回落库格式 obj:act。
func (p Perm) String() string {
	return p.Obj + ":" + p.Act
}

// Parse 解析 obj:act。
func Parse(value string) (Perm, error) {
	obj, act, ok := strings.Cut(value, ":")
	if !ok {
		return Perm{}, fmt.Errorf("authz: invalid permission %q, want obj:act", value)
	}

	perm := Perm{Obj: obj, Act: act}
	if err := perm.Validate(); err != nil {
		return Perm{}, err
	}

	return perm, nil
}

// Validate 校验权限点格式：Obj 允许点分层，Act 用短动词。
func (p Perm) Validate() error {
	if p.Obj == "" || p.Act == "" {
		return fmt.Errorf("authz: permission %q must have object and action", p.String())
	}
	if strings.Contains(p.Obj, ":") || strings.Contains(p.Act, ":") {
		return fmt.Errorf("authz: permission %q must not contain ':' inside segments", p.String())
	}
	if strings.ContainsAny(p.Obj, " \t") || strings.ContainsAny(p.Act, " \t") {
		return fmt.Errorf("authz: permission %q must not contain whitespace", p.String())
	}
	if strings.Contains(p.Act, "_") {
		return fmt.Errorf("authz: action %q must be a short verb without underscores", p.Act)
	}
	if p.Obj == "*" && p.Act != "*" {
		return fmt.Errorf("authz: wildcard object requires wildcard action")
	}

	return nil
}

var (
	registryMu sync.RWMutex
	registry   = make(map[Perm]struct{})
)

// Register 登记代码里声明的权限点，供启动校验与内置角色同步使用。
// 由各模块在装配阶段调用，不在 init() 里注册。
func Register(perms ...Perm) error {
	for _, perm := range perms {
		if err := perm.Validate(); err != nil {
			return err
		}
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	for _, perm := range perms {
		registry[perm] = struct{}{}
	}

	return nil
}

// All 返回已登记的全部权限点（含通配），按字符串排序。
func All() []Perm {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]Perm, 0, len(registry))
	for perm := range registry {
		out = append(out, perm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })

	return out
}

// Known 判断权限点是否已在代码里登记。
func Known(perm Perm) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if perm == Wildcard() {
		return true
	}
	_, ok := registry[perm]
	return ok
}

// ResetRegistry 仅供测试重建注册表。
func ResetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = make(map[Perm]struct{})
}
