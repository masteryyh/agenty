package builtin

import (
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
)

func RegisterAll(registry *agentloop.Registry) error {
	if registry == nil {
		return fmt.Errorf("builtin: registry must not be nil")
	}

	fileSystem := &fileSystem{}
	tools := []agentloop.Tool{
		&shellTool{},
		&readFileTool{fileSystem: fileSystem},
		&applyPatchTool{fileSystem: fileSystem},
		&grepTool{fileSystem: fileSystem},
		&globTool{fileSystem: fileSystem},
		&listTool{fileSystem: fileSystem},
	}
	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			return fmt.Errorf("builtin: register tool %q: %w", tool.Definition().Name, err)
		}
	}
	return nil
}
