package tenant

// CreateInput 是开通租户的入参。
type CreateInput struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	Timezone         string `json:"timezone"`
	Locale           string `json:"locale"`
	Domain           string `json:"domain"`
	Subdomain        string `json:"subdomain"`
	PlanCode         string `json:"plan_code"`
	ContactName      string `json:"contact_name"`
	ContactPhone     string `json:"contact_phone"`
	ContactEmail     string `json:"contact_email"`
	AdminEmail       string `json:"admin_email"`
	AdminDisplayName string `json:"admin_display_name"`
	AdminPassword    string `json:"admin_password"`
}

// ListFilter 是列表过滤条件。
type ListFilter struct {
	Status string
	Page   int
	Size   int
}

// UpdateStatusInput 是变更租户状态的入参。
type UpdateStatusInput struct {
	Status string `json:"status"`
}
