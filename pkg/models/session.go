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
	"github.com/masteryyh/agenty/pkg/db"
)

type ChatSessionDto struct {
	ID            uuid.UUID        `json:"id"`
	AgentID       uuid.UUID        `json:"agentId"`
	TokenConsumed int64            `json:"tokenConsumed"`
	Messages      []ChatMessageDto `json:"messages"`
	LastUsedModel uuid.UUID        `json:"lastUsedModel"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
}

func ChatSessionRowToDto(row db.ChatSession, messages []ChatMessageDto) *ChatSessionDto {
	dto := &ChatSessionDto{
		ID:            row.ID,
		AgentID:       row.AgentID,
		TokenConsumed: row.TokenConsumed,
		LastUsedModel: row.LastUsedModel,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
	if messages != nil {
		dto.Messages = messages
	}
	return dto
}

type CreateSessionDto struct {
	AgentID uuid.UUID `json:"agentId" binding:"required"`
}
