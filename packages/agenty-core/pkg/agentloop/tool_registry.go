package agentloop

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

var _ ToolRuntime = (*Registry)(nil)

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("agentloop: register nil tool")
	}

	name := tool.Definition().Name
	if name == "" {
		return fmt.Errorf("agentloop: register tool with empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("agentloop: tool %q is already registered", name)
	}
	r.tools[name] = tool

	return nil
}

func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tools, name)
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) Definitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	definitions := make([]ToolDefinition, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, r.tools[name].Definition())
	}

	return definitions
}

func (r *Registry) ExecuteBatch(
	ctx context.Context,
	callContext CallContext,
	calls []conversation.ToolUseBlock,
) []conversation.ToolResultBlock {
	results := make([]conversation.ToolResultBlock, len(calls))

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(calls))
	for index, call := range calls {
		go func() {
			defer waitGroup.Done()
			results[index] = r.execute(ctx, callContext, call)
		}()
	}
	waitGroup.Wait()

	return results
}

func (r *Registry) execute(
	ctx context.Context,
	callContext CallContext,
	call conversation.ToolUseBlock,
) (result conversation.ToolResultBlock) {
	result.ToolUseID = call.ID

	defer func() {
		if recovered := recover(); recovered != nil {
			result.Content = conversation.Text(fmt.Sprintf("tool %q panicked: %v", call.Name, recovered))
			result.IsError = true
		}
	}()

	tool, ok := r.Get(call.Name)
	if !ok {
		result.Content = conversation.Text(fmt.Sprintf("tool %q is not registered", call.Name))
		result.IsError = true
		return result
	}

	content, err := tool.Execute(ctx, callContext, call.Input)
	if err != nil {
		result.Content = conversation.Text(fmt.Sprintf("tool %q failed: %v", call.Name, err))
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
