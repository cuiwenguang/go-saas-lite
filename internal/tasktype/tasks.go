// Package tasktype 定义异步任务类型与 payload，供生产者和消费者共用。
package tasktype

// 任务类型常量。
const (
	SMSSend              = "sms.send"
	FileDelete           = "file.delete"
	TenantPurge          = "tenant.purge"
	TenantProvisionRetry = "tenant.provision.retry"
	DemoHeartbeat        = "demo.heartbeat"
)

// SMSSendPayload 是短信发送任务。
type SMSSendPayload struct {
	Phone string `json:"phone"`
	Scene string `json:"scene"`
	Code  string `json:"code"`
}

// FileDeletePayload 是对象删除任务。
type FileDeletePayload struct {
	Key string `json:"key"`
}

// TenantPurgePayload 是租户数据清理任务。
type TenantPurgePayload struct {
	TenantID string `json:"tenant_id"`
}

// TenantProvisionRetryPayload 是开通续跑任务。
type TenantProvisionRetryPayload struct {
	TenantID string `json:"tenant_id"`
}

// HeartbeatPayload 是定时任务示例。
type HeartbeatPayload struct {
	At string `json:"at"`
}
