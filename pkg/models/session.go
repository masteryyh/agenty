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
	ID            uuid.UUID  `gorm:"type:uuid;default:uuidv7();primaryKey;index:idx_session_id_deleted_at"`
	TokenConsumed int64      `gorm:"not null;default:0"`
	CreatedAt     time.Time  `gorm:"autoCreateTime:milli"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime:milli"`
	DeletedAt     *time.Time `gorm:"index:idx_session_id_deleted_at"`
}

func (ChatSession) TableName() string {
	return "chat_sessions"
}

type ChatSessionDto struct {
	ID            uuid.UUID        `json:"id"`
	TokenConsumed int64            `json:"tokenConsumed"`
	Messages      []ChatMessageDto `json:"messages"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
}
