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

package models

import (
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type MCPTransportType string

const (
	MCPTransportStdio          MCPTransportType = "stdio"
	MCPTransportSSE            MCPTransportType = "sse"
	MCPTransportStreamableHTTP MCPTransportType = "streamable-http"
)

type MCPServer struct {
	ID        uuid.UUID        `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	Name      string           `gorm:"type:varchar(255);not null;uniqueIndex:idx_mcp_servers_name,where:deleted_at IS NULL"`
	Transport MCPTransportType `gorm:"type:varchar(50);not null"`
	Enabled   bool             `gorm:"not null;default:true"`
	Command   string           `gorm:"type:varchar(512)"`
	Args      datatypes.JSON   `gorm:"type:jsonb"`
	Env       datatypes.JSON   `gorm:"type:jsonb"`
	URL       string           `gorm:"type:varchar(512)"`
	Headers   datatypes.JSON   `gorm:"type:jsonb"`
	CreatedAt time.Time        `gorm:"autoCreateTime"`
	UpdatedAt time.Time        `gorm:"autoUpdateTime"`
	DeletedAt *time.Time
}

func (MCPServer) TableName() string {
	return "mcp_servers"
}

func (s *MCPServer) ToDto() *MCPServerDto {
	dto := &MCPServerDto{
		ID:        s.ID,
		Name:      s.Name,
		Transport: s.Transport,
		Enabled:   s.Enabled,
		Command:   s.Command,
		URL:       s.URL,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}

	if len(s.Args) > 0 {
		_ = json.Unmarshal(s.Args, &dto.Args)
	}
	if len(s.Env) > 0 {
		_ = json.Unmarshal(s.Env, &dto.Env)
	}
	if len(s.Headers) > 0 {
		_ = json.Unmarshal(s.Headers, &dto.Headers)
	}

	return dto
}

type MCPServerDto struct {
	ID        uuid.UUID         `json:"id"`
	Name      string            `json:"name"`
	Transport MCPTransportType  `json:"transport"`
	Enabled   bool              `json:"enabled"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Status    string            `json:"status,omitempty"`
	Tools     []string          `json:"tools,omitempty"`
	Error     string            `json:"error,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type CreateMCPServerDto struct {
	Name      string            `json:"name" binding:"required"`
	Transport MCPTransportType  `json:"transport" binding:"required,oneof=stdio sse streamable-http"`
	Enabled   *bool             `json:"enabled"`
	Command   string            `json:"command" binding:"required_if=Transport stdio"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	URL       string            `json:"url" binding:"required_if=Transport sse,required_if=Transport streamable-http"`
	Headers   map[string]string `json:"headers"`
}

type UpdateMCPServerDto struct {
	Name      string            `json:"name"`
	Transport MCPTransportType  `json:"transport" binding:"omitempty,oneof=stdio sse streamable-http"`
	Enabled   *bool             `json:"enabled"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
}
