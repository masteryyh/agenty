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

package mcp

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/utils"
)

type MCPTool struct {
	serverName string
	toolName   string
	tool       mcp.Tool
	client     client.MCPClient
}

func NewMCPTool(serverName string, tool mcp.Tool, c client.MCPClient) *MCPTool {
	sanitizedServerName := utils.SanitizeName(serverName, "mcpserver")
	sanitizedToolName := utils.SanitizeName(tool.Name, "tool")

	return &MCPTool{
		serverName: sanitizedServerName,
		toolName:   fmt.Sprintf("mcp_%s_%s", sanitizedServerName, sanitizedToolName),
		tool:       tool,
		client:     c,
	}
}

func (t *MCPTool) Definition() tools.ToolDefinition {
	props := make(map[string]tools.ParameterProperty)
	if t.tool.InputSchema.Properties != nil {
		for name, raw := range t.tool.InputSchema.Properties {
			props[name] = convertProperty(raw)
		}
	}

	return tools.ToolDefinition{
		Name:        t.toolName,
		Description: fmt.Sprintf("[MCP:%s] %s", t.serverName, t.tool.Description),
		Parameters: tools.ToolParameters{
			Type:       "object",
			Properties: props,
			Required:   t.tool.InputSchema.Required,
		},
	}
}

func (t *MCPTool) Execute(ctx context.Context, _ tools.ToolCallContext, arguments string) (string, error) {
	var args map[string]any
	if arguments != "" {
		if err := json.UnmarshalString(arguments, &args); err != nil {
			return "", fmt.Errorf("failed to parse arguments: %w", err)
		}
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = t.tool.Name
	req.Params.Arguments = args

	result, err := t.client.CallTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("mcp tool call failed: %w", err)
	}

	if result.IsError {
		msg := extractTextContent(result.Content)
		if strings.TrimSpace(msg) != "" {
			return msg, fmt.Errorf("mcp tool returned error: %s", msg)
		}
		return msg, fmt.Errorf("mcp tool returned error")
	}

	return extractTextContent(result.Content), nil
}

type MCPResourceTool struct {
	serverName string
	client     client.MCPClient
	resources  []mcp.Resource
}

func NewMCPResourceTool(serverName string, c client.MCPClient, resources []mcp.Resource) *MCPResourceTool {
	return &MCPResourceTool{
		serverName: serverName,
		client:     c,
		resources:  resources,
	}
}

func (t *MCPResourceTool) Definition() tools.ToolDefinition {
	uris := make([]string, len(t.resources))
	for i, r := range t.resources {
		uris[i] = r.URI
	}

	return tools.ToolDefinition{
		Name:        fmt.Sprintf("mcp_%s_read_resource", t.serverName),
		Description: fmt.Sprintf("[MCP:%s] Read a resource. Available: %s", t.serverName, strings.Join(uris, ", ")),
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"uri": {
					Type:        "string",
					Description: "The URI of the resource to read",
				},
			},
			Required: []string{"uri"},
		},
	}
}

func (t *MCPResourceTool) Execute(ctx context.Context, _ tools.ToolCallContext, arguments string) (string, error) {
	var args struct {
		URI string `json:"uri"`
	}
	if err := json.UnmarshalString(arguments, &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = args.URI

	result, err := t.client.ReadResource(ctx, req)
	if err != nil {
		return "", fmt.Errorf("mcp resource read failed: %w", err)
	}

	var sb strings.Builder
	for _, content := range result.Contents {
		if tc, ok := mcp.AsTextResourceContents(content); ok {
			sb.WriteString(tc.Text)
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

func convertProperty(raw any) tools.ParameterProperty {
	prop := tools.ParameterProperty{Type: "string"}

	m, ok := raw.(map[string]any)
	if !ok {
		return prop
	}

	if t, ok := m["type"].(string); ok {
		prop.Type = t
	}
	if d, ok := m["description"].(string); ok {
		prop.Description = d
	}
	if items, ok := m["items"]; ok {
		converted := convertProperty(items)
		prop.Items = &converted
	}

	return prop
}

func extractTextContent(contents []mcp.Content) string {
	var sb strings.Builder
	for _, c := range contents {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String())
}
