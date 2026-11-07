package file

import "time"

// Output 是文件元数据出参。
type Output struct {
	ID           string    `json:"id"`
	Key          string    `json:"key"`
	Biz          string    `json:"biz"`
	OriginalName string    `json:"original_name"`
	ContentType  string    `json:"content_type"`
	SizeBytes    int64     `json:"size_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}
