// Package apidocs 嵌入 OpenAPI 规范，供 HTTP 服务直接输出。
package apidocs

import _ "embed"

// OpenAPI 是 docs/api/openapi.yaml 的内容。
//
//go:embed openapi.yaml
var OpenAPI []byte
