package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (m *Agent) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *AgentGatewayBinding) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *ChatMessage) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *ChatRoundTokenUsage) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *ChatCompaction) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *ChatSession) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *GatewayConversation) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *GatewayMessageDelivery) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *KnowledgeBaseData) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *KnowledgeItem) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *MCPServer) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *Model) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *ModelProvider) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *SessionSkill) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func (m *Skill) BeforeCreate(*gorm.DB) error {
	return ensureUUID(&m.ID)
}

func ensureUUID(id *uuid.UUID) error {
	if *id != uuid.Nil {
		return nil
	}
	newID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	*id = newID
	return nil
}
