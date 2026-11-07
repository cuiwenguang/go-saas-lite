package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var schemaCache sync.Map // map[string]*jsonschema.Schema

// validateInput 按工具声明的 JSON Schema 校验 arguments。
func validateInput(schema json.RawMessage, args json.RawMessage) error {
	raw := strings.TrimSpace(string(schema))
	if raw == "" || raw == "{}" || raw == "null" {
		return nil
	}

	compiled, err := compileSchema(raw)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}

	payload := strings.TrimSpace(string(args))
	if payload == "" {
		payload = "{}"
	}

	var value any
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}

	return compiled.Validate(value)
}

func compileSchema(raw string) (*jsonschema.Schema, error) {
	if cached, ok := schemaCache.Load(raw); ok {
		return cached.(*jsonschema.Schema), nil
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", bytes.NewReader([]byte(raw))); err != nil {
		return nil, err
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, err
	}

	schemaCache.Store(raw, schema)

	return schema, nil
}
