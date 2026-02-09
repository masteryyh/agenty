/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tools

import (
	"context"
	"sort"
	"sync"

	"github.com/masteryyh/agenty/pkg/models"
	"github.com/samber/lo"
)

type ParameterProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ToolParameters struct {
	Type       string                       `json:"type"`
	Properties map[string]ParameterProperty `json:"properties"`
	Required   []string                     `json:"required"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, arguments string) (string, error)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

func GetRegistry() *Registry {
	registryOnce.Do(func() {
		globalRegistry = &Registry{
			tools: make(map[string]Tool),
		}
	})
	return globalRegistry
}

func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Definition().Name] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

func (r *Registry) Definitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := lo.Keys(r.tools)
	sort.Strings(keys)

	result := make([]ToolDefinition, 0, len(r.tools))
	for _, key := range keys {
		result = append(result, r.tools[key].Definition())
	}
	return result
}

func (r *Registry) Execute(ctx context.Context, call models.ToolCall) models.ToolResult {
	tool, ok := r.Get(call.Name)
	if !ok {
		return models.ToolResult{
			CallID:  call.ID,
			Name:    call.Name,
			Content: "tool not found: " + call.Name,
			IsError: true,
		}
	}

	content, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return models.ToolResult{
			CallID:  call.ID,
			Name:    call.Name,
			Content: "error: " + err.Error(),
			IsError: true,
		}
	}

	return models.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: content,
	}
}
