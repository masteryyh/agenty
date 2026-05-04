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

	"github.com/google/uuid"
)

type ChatSession struct {
	ID            uuid.UUID
	AgentID       uuid.UUID
	TokenConsumed int64
	LastUsedModel uuid.UUID
	Cwd           *string
	AgentsMD      *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

func (ChatSession) TableName() string {
	return "chat_sessions"
}

func (m *ChatSession) ToDto(messages []ChatMessageDto) *ChatSessionDto {
	dto := &ChatSessionDto{
		ID:            m.ID,
		AgentID:       m.AgentID,
		TokenConsumed: m.TokenConsumed,
		LastUsedModel: m.LastUsedModel,
		Cwd:           m.Cwd,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}

	if messages != nil {
		dto.Messages = messages
	}
	return dto
}

type TodoItemDto struct {
	ID      int    `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type ChatSessionDto struct {
	ID            uuid.UUID        `json:"id"`
	AgentID       uuid.UUID        `json:"agentId"`
	TokenConsumed int64            `json:"tokenConsumed"`
	Messages      []ChatMessageDto `json:"messages"`
	LastUsedModel uuid.UUID        `json:"lastUsedModel"`
	Todos         []TodoItemDto    `json:"todos,omitempty"`
	Cwd           *string          `json:"cwd,omitempty"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
}

type CreateSessionDto struct {
	AgentID uuid.UUID `json:"agentId" binding:"required"`
}

type SetSessionCwdDto struct {
	Cwd      *string `json:"cwd"`
	AgentsMD *string `json:"agentsMD"`
}
