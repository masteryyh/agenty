package builtin_test

import (
	"context"
	"testing"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/agentloop/builtin"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

func TestRegisterAll(t *testing.T) {
	t.Parallel()

	registry := agentloop.NewRegistry()
	if err := builtin.RegisterAll(registry); err != nil {
		t.Fatal(err)
	}

	wantNames := []string{
		"delete_file",
		"glob",
		"grep",
		"ls",
		"patch_file",
		"read_file",
		"shell",
		"write_file",
	}
	definitions := registry.Definitions()
	if len(definitions) != len(wantNames) {
		t.Fatalf("definitions = %d, want %d", len(definitions), len(wantNames))
	}
	for index, wantName := range wantNames {
		definition := definitions[index]
		if definition.Name != wantName {
			t.Errorf("definition %d name = %q, want %q", index, definition.Name, wantName)
		}
		if definition.InputSchema.Type != agentloop.JSONSchemaTypeObject {
			t.Errorf("definition %q schema type = %q, want object", definition.Name, definition.InputSchema.Type)
		}
		additional := definition.InputSchema.AdditionalProperties
		if additional == nil || additional.Allowed == nil || *additional.Allowed {
			t.Errorf("definition %q additionalProperties = %#v, want false", definition.Name, additional)
		}
		if _, err := agentloop.ToolSchemaMap(definition.InputSchema); err != nil {
			t.Errorf("definition %q schema conversion: %v", definition.Name, err)
		}
	}
}

func TestRegisterAllRejectsInvalidRegistryState(t *testing.T) {
	t.Parallel()

	if err := builtin.RegisterAll(nil); err == nil {
		t.Error("RegisterAll(nil) succeeded")
	}

	registry := agentloop.NewRegistry()
	if err := builtin.RegisterAll(registry); err != nil {
		t.Fatal(err)
	}
	if err := builtin.RegisterAll(registry); err == nil {
		t.Error("second RegisterAll call succeeded")
	}
}

func newRegistry(t *testing.T) *agentloop.Registry {
	t.Helper()

	registry := agentloop.NewRegistry()
	if err := builtin.RegisterAll(registry); err != nil {
		t.Fatal(err)
	}
	return registry
}

func executeTool(
	t *testing.T,
	registry *agentloop.Registry,
	name string,
	cwd string,
	arguments string,
) (string, error) {
	t.Helper()

	tool, ok := registry.Get(name)
	if !ok {
		t.Fatalf("tool %q is not registered", name)
	}
	content, err := tool.Execute(
		context.Background(),
		agentloop.CallContext{Cwd: cwd},
		[]byte(arguments),
	)
	if err != nil {
		return "", err
	}
	if len(content) != 1 {
		t.Fatalf("tool %q content blocks = %d, want 1", name, len(content))
	}
	block, ok := content[0].(conversation.TextBlock)
	if !ok {
		t.Fatalf("tool %q block = %T, want conversation.TextBlock", name, content[0])
	}
	return block.Text, nil
}

func decodeResult[T any](t *testing.T, encoded string) T {
	t.Helper()

	var result T
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode tool result: %v\nresult: %s", err, encoded)
	}
	return result
}
