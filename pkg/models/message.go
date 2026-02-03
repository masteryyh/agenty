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

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type ChatMessage struct {
	ID        uuid.UUID   `gorm:"type:uuid;primaryKey;default:uuidv7();index:idx_message_id_deleted_at"`
	SessionID uuid.UUID   `gorm:"type:uuid;not null;index:idx_message_session_id_deleted_at"`
	Role      MessageRole `gorm:"type:varchar(50);not null"`
	Content   string      `gorm:"type:text;not null"`
	CreatedAt time.Time   `gorm:"autoCreateTime:milli"`
	DeletedAt *time.Time  `gorm:"index:idx_message_id_deleted_at;index:idx_message_session_id_deleted_at"`
}

func (ChatMessage) TableName() string {
	return "chat_messages"
}

type ChatMessageDto struct {
	ID        uuid.UUID   `json:"id"`
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	CreatedAt time.Time   `json:"createdAt"`
}

type ChatDto struct {
	Message string `json:"message" binding:"required"`
}
