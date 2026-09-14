// Package tools owns the mutable, dynamically updated tool registry used by
// infrastructure integrations. AgentLoop only depends on the ToolRuntime port.
package tools

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"sync"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]agentloop.Tool
}

// Registrar is the narrow registration port shared by built-in and integration
// tool providers.
type Registrar interface {
	Register(agentloop.Tool) error
}

// Port is the complete mutable registry contract used by infrastructure
// integrations such as MCP.
type Port interface {
	agentloop.ToolRuntime
	Registrar
	Unregister(string)
	Get(string) (agentloop.Tool, bool)
	SnapshotToolRuntime() agentloop.ToolRuntime
}

// Combine returns an immutable view over the supplied runtimes. Definitions
// and execution are resolved from the same captured set, so callers can keep
// the result for the lifetime of a round while the source registries change.
func Combine(runtimes ...agentloop.ToolRuntime) agentloop.ToolRuntime {
	components := make([]agentloop.ToolRuntime, 0, len(runtimes))
	for _, runtime := range runtimes {
		if runtime != nil {
			components = append(components, runtime)
		}
	}
	if len(components) == 0 {
		return nil
	}
	if len(components) == 1 {
		return components[0]
	}

	owners := make(map[string]int)
	definitions := make([]modelcall.ToolDefinition, 0)
	for componentIndex, runtime := range components {
		for _, definition := range runtime.Definitions() {
			if definition.Name == "" {
				continue
			}
			if _, exists := owners[definition.Name]; exists {
				continue
			}
			owners[definition.Name] = componentIndex
			definitions = append(definitions, definition)
		}
	}
	sort.Slice(definitions, func(left, right int) bool {
		return definitions[left].Name < definitions[right].Name
	})
	return &combinedRuntime{components: components, owners: owners, definitions: definitions}
}

type combinedRuntime struct {
	components  []agentloop.ToolRuntime
	owners      map[string]int
	definitions []modelcall.ToolDefinition
}

func (runtime *combinedRuntime) Definitions() []modelcall.ToolDefinition {
	return append([]modelcall.ToolDefinition(nil), runtime.definitions...)
}

func (runtime *combinedRuntime) ExecuteBatch(
	ctx context.Context,
	callContext agentloop.CallContext,
	calls []conversation.ToolUseBlock,
) []conversation.ToolResultBlock {
	results := make([]conversation.ToolResultBlock, len(calls))
	groups := make(map[int][]int)
	for index, call := range calls {
		componentIndex, ok := runtime.owners[call.Name]
		if !ok {
			results[index] = conversation.ToolResultBlock{
				ToolUseID: call.ID,
				Content:   conversation.Text(fmt.Sprintf("tool %q is not registered", call.Name)),
				IsError:   true,
			}
			continue
		}
		groups[componentIndex] = append(groups[componentIndex], index)
	}

	var waitGroup sync.WaitGroup
	for componentIndex, indexes := range groups {
		waitGroup.Add(1)
		go func(componentIndex int, indexes []int) {
			defer waitGroup.Done()
			callsForComponent := make([]conversation.ToolUseBlock, 0, len(indexes))
			for _, index := range indexes {
				callsForComponent = append(callsForComponent, calls[index])
			}
			componentResults := runtime.components[componentIndex].ExecuteBatch(ctx, callContext, callsForComponent)
			for resultIndex, index := range indexes {
				if resultIndex < len(componentResults) {
					results[index] = componentResults[resultIndex]
					continue
				}
				results[index] = conversation.ToolResultBlock{
					ToolUseID: calls[index].ID,
					Content:   conversation.Text(fmt.Sprintf("tool %q returned no result", calls[index].Name)),
					IsError:   true,
				}
			}
		}(componentIndex, indexes)
	}
	waitGroup.Wait()
	return results
}

var _ Port = (*Registry)(nil)

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]agentloop.Tool)}
}

func (registry *Registry) Register(tool agentloop.Tool) error {
	if tool == nil {
		return fmt.Errorf("tool registry: register nil tool")
	}
	name := tool.Definition().Name
	if name == "" {
		return fmt.Errorf("tool registry: register tool with empty name")
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.tools[name]; exists {
		return fmt.Errorf("tool registry: tool %q is already registered", name)
	}
	registry.tools[name] = tool
	return nil
}

func (registry *Registry) Unregister(name string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	delete(registry.tools, name)
}

func (registry *Registry) Get(name string) (agentloop.Tool, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	tool, ok := registry.tools[name]
	return tool, ok
}

func (registry *Registry) Definitions() []modelcall.ToolDefinition {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	names := make([]string, 0, len(registry.tools))
	for name := range registry.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	definitions := make([]modelcall.ToolDefinition, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, registry.tools[name].Definition())
	}
	return definitions
}

func (registry *Registry) SnapshotToolRuntime() agentloop.ToolRuntime {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	snapshot := &Registry{tools: make(map[string]agentloop.Tool, len(registry.tools))}
	maps.Copy(snapshot.tools, registry.tools)
	return snapshot
}

func (registry *Registry) ExecuteBatch(
	ctx context.Context,
	callContext agentloop.CallContext,
	calls []conversation.ToolUseBlock,
) []conversation.ToolResultBlock {
	results := make([]conversation.ToolResultBlock, len(calls))
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(calls))
	for index, call := range calls {
		go func() {
			defer waitGroup.Done()
			results[index] = registry.execute(ctx, callContext, call)
		}()
	}
	waitGroup.Wait()
	return results
}

func (registry *Registry) execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	call conversation.ToolUseBlock,
) (result conversation.ToolResultBlock) {
	result.ToolUseID = call.ID
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Content = conversation.Text(fmt.Sprintf("tool %q panicked: %v", call.Name, recovered))
			result.IsError = true
		}
	}()

	tool, ok := registry.Get(call.Name)
	if !ok {
		result.Content = conversation.Text(fmt.Sprintf("tool %q is not registered", call.Name))
		result.IsError = true
		return result
	}
	content, err := tool.Execute(ctx, callContext, call.Input)
	if err != nil {
		var structured *ToolExecutionError
		if errors.As(err, &structured) && structured != nil && len(structured.Content) > 0 {
			result.Content = structured.Content
		} else {
			result.Content = conversation.Text(fmt.Sprintf("tool %q failed: %v", call.Name, err))
		}
		result.IsError = true
		return result
	}
	for index, block := range content {
		output, ok := block.(conversation.ShellCallOutputBlock)
		if !ok {
			continue
		}
		output.CallID = call.ID
		content[index] = output
	}
	result.Content = content
	return result
}
