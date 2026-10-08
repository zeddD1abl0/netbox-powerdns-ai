package api

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestReference(t *testing.T) {
	var b strings.Builder
	if err := WriteReference(&b); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"`
		}
		Components struct{ Schemas map[string]any }
	}
	if err := yaml.Unmarshal(Spec(), &doc); err != nil {
		t.Fatal(err)
	}
	// Every operation and every schema has its section.
	for path, methods := range doc.Paths {
		for method, op := range methods {
			if !strings.Contains(page, "### `"+strings.ToUpper(method)+" /api"+path+"`") || !strings.Contains(page, "Operation `"+op.OperationID+"`") {
				t.Errorf("no section for %s %s", method, path)
			}
		}
	}
	for name := range doc.Components.Schemas {
		if !strings.Contains(page, "### `"+name+"`") {
			t.Errorf("no section for the schema %s", name)
		}
	}
	// Every field has a description, its own or its schema's.
	for line := range strings.Lines(page) {
		if strings.HasSuffix(strings.TrimSpace(line), "|  |") {
			t.Errorf("a row without a description: %s", line)
		}
	}
}
