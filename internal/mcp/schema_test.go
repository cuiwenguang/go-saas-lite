package mcp

import (
	"encoding/json"
	"testing"
)

func TestValidateInput(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {"order_id": {"type": "string"}},
		"required": ["order_id"],
		"additionalProperties": false
	}`)

	if err := validateInput(schema, json.RawMessage(`{"order_id":"o-1"}`)); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
	if err := validateInput(schema, json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing required field should be rejected")
	}
	if err := validateInput(schema, json.RawMessage(`{"order_id":1}`)); err == nil {
		t.Fatal("wrong type should be rejected")
	}
	if err := validateInput(schema, json.RawMessage(`{"order_id":"o-1","extra":true}`)); err == nil {
		t.Fatal("additional property should be rejected")
	}
	if err := validateInput(json.RawMessage(`{}`), json.RawMessage(`{"anything":1}`)); err != nil {
		t.Fatalf("empty schema should accept anything: %v", err)
	}
}
