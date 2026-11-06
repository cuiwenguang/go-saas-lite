package members

import "time"

// ListFilter 是成员列表过滤条件。
type ListFilter struct {
	Status  string
	Keyword string
	Page    int
	Size    int
}

// CreateInput 是新建成员入参；Password 为空时生成一次性初始密码。
type CreateInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

// UpdateInput 是可修改字段；nil 表示不改。
type UpdateInput struct {
	DisplayName *string `json:"display_name"`
	Status      *string `json:"status"`
}

// SetRolesInput 是替换角色绑定入参。
type SetRolesInput struct {
	Codes []string `json:"codes"`
}

// UserOutput 是成员出参。
type UserOutput struct {
	ID                 string    `json:"id"`
	Email              string    `json:"email"`
	DisplayName        string    `json:"display_name"`
	Status             string    `json:"status"`
	Roles              []string  `json:"roles"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
}

// CredentialOutput 携带一次性初始密码。
type CredentialOutput struct {
	User            UserOutput `json:"user"`
	InitialPassword string     `json:"initial_password"`
}

// RoleOutput 是内置角色出参（只读）。
type RoleOutput struct {
	ID          string   `json:"id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}
