package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"gorm.io/gorm"
)

type GatewayService struct {
	db *gorm.DB
}

type GatewayReloadFunc func(context.Context, []models.GatewayChannel) error

var (
	gatewayService *GatewayService
	gatewayOnce    sync.Once
)

func GetGatewayService() *GatewayService {
	gatewayOnce.Do(func() {
		gatewayService = &GatewayService{
			db: conn.GetDB(),
		}
	})
	return gatewayService
}

func (s *GatewayService) ListChannels(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.GatewayChannelDto], error) {
	var channels []models.GatewayChannel
	var total int64

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Order("created_at DESC").
			Offset((request.Page - 1) * request.PageSize).
			Limit(request.PageSize).
			Find(&channels).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.GatewayChannel{}).Count(&total).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		slog.ErrorContext(ctx, "failed to list gateway channels", "error", err)
		return nil, err
	}

	dtos := make([]models.GatewayChannelDto, 0, len(channels))
	for i := range channels {
		dtos = append(dtos, *channels[i].ToDto())
	}

	return &pagination.PagedResponse[models.GatewayChannelDto]{
		Total:    total,
		Page:     request.Page,
		PageSize: request.PageSize,
		Data:     dtos,
	}, nil
}

func (s *GatewayService) GetChannel(ctx context.Context, channelID string) (*models.GatewayChannelDto, error) {
	channel, err := s.getChannelModel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return channel.ToDto(), nil
}

func (s *GatewayService) ListEnabledChannelConfigs(ctx context.Context) ([]models.GatewayChannel, error) {
	return s.listEnabledChannelConfigs(ctx, s.db)
}

func (s *GatewayService) listEnabledChannelConfigs(ctx context.Context, db *gorm.DB) ([]models.GatewayChannel, error) {
	var channels []models.GatewayChannel
	if err := db.WithContext(ctx).
		Order("created_at ASC").
		Where("enabled = TRUE").
		Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

func (s *GatewayService) CreateChannel(ctx context.Context, dto *models.CreateGatewayChannelDto) (*models.GatewayChannelDto, error) {
	return s.CreateChannelWithReload(ctx, dto, nil)
}

func (s *GatewayService) CreateChannelWithReload(ctx context.Context, dto *models.CreateGatewayChannelDto, reload GatewayReloadFunc) (*models.GatewayChannelDto, error) {
	channelID := strings.TrimSpace(dto.ID)
	if channelID == "" {
		return nil, customerrors.ErrInvalidParams
	}
	if channelID == "cli" {
		return nil, customerrors.ErrInvalidParams
	}
	enabled := true
	if dto.Enabled != nil {
		enabled = *dto.Enabled
	}

	channel := &models.GatewayChannel{
		ID:             channelID,
		Type:           models.ChannelType(strings.TrimSpace(dto.Type)),
		AccountID:      strings.TrimSpace(dto.AccountID),
		Enabled:        enabled,
		Required:       dto.Required,
		SendReasoning:  dto.SendReasoning,
		SendToolEvents: dto.SendToolEvents,
		RequireMention: dto.RequireMention,
	}
	if err := s.applyChannelConfig(channel, dto.Discord); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.validateChannelModel(ctx, tx, channel, ""); err != nil {
			return err
		}
		if err := tx.Create(channel).Error; err != nil {
			if isDuplicateKeyError(err) {
				return gatewayChannelCreateDuplicateError(err)
			}
			return err
		}
		return s.reloadActiveGateway(ctx, tx, reload)
	}); err != nil {
		slog.ErrorContext(ctx, "failed to create gateway channel", "error", err, "channelId", channel.ID)
		return nil, err
	}
	return channel.ToDto(), nil
}

func (s *GatewayService) UpdateChannel(ctx context.Context, channelID string, dto *models.UpdateGatewayChannelDto) (*models.GatewayChannelDto, error) {
	return s.UpdateChannelWithReload(ctx, channelID, dto, nil)
}

func (s *GatewayService) UpdateChannelWithReload(ctx context.Context, channelID string, dto *models.UpdateGatewayChannelDto, reload GatewayReloadFunc) (*models.GatewayChannelDto, error) {
	channel, err := s.getChannelModel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	oldType := channel.Type
	oldAccountID := channel.AccountID

	if t := strings.TrimSpace(dto.Type); t != "" {
		channel.Type = models.ChannelType(t)
	}
	if accountID := strings.TrimSpace(dto.AccountID); accountID != "" {
		channel.AccountID = accountID
	}
	if dto.Enabled != nil {
		channel.Enabled = *dto.Enabled
	}
	if dto.Required != nil {
		channel.Required = *dto.Required
	}
	if dto.SendReasoning != nil {
		channel.SendReasoning = *dto.SendReasoning
	}
	if dto.SendToolEvents != nil {
		channel.SendToolEvents = *dto.SendToolEvents
	}
	if dto.RequireMention != nil {
		channel.RequireMention = *dto.RequireMention
	}
	if dto.Discord != nil {
		if err := s.applyChannelConfig(channel, dto.Discord); err != nil {
			return nil, err
		}
	}
	updates := map[string]any{
		"type":             channel.Type,
		"account_id":       channel.AccountID,
		"enabled":          channel.Enabled,
		"required":         channel.Required,
		"send_reasoning":   channel.SendReasoning,
		"send_tool_events": channel.SendToolEvents,
		"require_mention":  channel.RequireMention,
		"config":           channel.Config,
		"updated_at":       conn.NowExpr(),
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.validateChannelModel(ctx, tx, channel, channelID); err != nil {
			return err
		}
		if err := tx.Model(&models.GatewayChannel{}).
			Where("id = ?", channelID).
			Updates(updates).Error; err != nil {
			if isDuplicateKeyError(err) {
				return customerrors.ErrGatewayChannelConflict
			}
			return err
		}
		if oldType != channel.Type || oldAccountID != channel.AccountID {
			if err := s.updateBindingsForChannelIdentity(ctx, tx, channelID, channel.Type, channel.AccountID); err != nil {
				return err
			}
		}
		return s.reloadActiveGateway(ctx, tx, reload)
	}); err != nil {
		slog.ErrorContext(ctx, "failed to update gateway channel", "error", err, "channelId", channelID)
		return nil, err
	}
	return s.GetChannel(ctx, channelID)
}

func (s *GatewayService) DeleteChannel(ctx context.Context, channelID string) error {
	return s.DeleteChannelWithReload(ctx, channelID, nil)
}

func (s *GatewayService) DeleteChannelWithReload(ctx context.Context, channelID string, reload GatewayReloadFunc) error {
	if _, err := s.getChannelModel(ctx, channelID); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", channelID).Delete(&models.GatewayMessageDelivery{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channelID).Delete(&models.GatewayConversation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channelID).Delete(&models.AgentGatewayBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", channelID).Delete(&models.GatewayChannel{}).Error; err != nil {
			return err
		}
		return s.reloadActiveGateway(ctx, tx, reload)
	}); err != nil {
		slog.ErrorContext(ctx, "failed to delete gateway channel", "error", err, "channelId", channelID)
		return err
	}
	return nil
}

func (s *GatewayService) ListBindings(ctx context.Context, agentID *uuid.UUID) ([]models.AgentGatewayBindingDto, error) {
	query := s.db.WithContext(ctx).Model(&models.AgentGatewayBinding{})
	if agentID != nil {
		query = query.Where("agent_id = ?", *agentID)
	}
	var bindings []models.AgentGatewayBinding
	if err := query.Order("created_at ASC").Find(&bindings).Error; err != nil {
		slog.ErrorContext(ctx, "failed to list gateway bindings", "error", err)
		return nil, err
	}
	out := make([]models.AgentGatewayBindingDto, 0, len(bindings))
	for i := range bindings {
		out = append(out, *bindings[i].ToDto())
	}
	return out, nil
}

func (s *GatewayService) CreateBinding(ctx context.Context, agentID uuid.UUID, dto *models.CreateAgentGatewayBindingDto) (*models.AgentGatewayBindingDto, error) {
	enabled := true
	if dto.Enabled != nil {
		enabled = *dto.Enabled
	}

	binding := &models.AgentGatewayBinding{}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		channel, err := s.validateBindingInput(ctx, tx, agentID, dto.ChannelID, dto.DefaultModelID)
		if err != nil {
			return err
		}
		binding.AgentID = agentID
		binding.ChannelID = channel.ID
		binding.ChannelType = channel.Type
		binding.AccountID = channel.AccountID
		binding.DefaultModelID = dto.DefaultModelID
		binding.Enabled = enabled
		if err := s.ensureBindingUniqueness(ctx, tx, agentID, binding.ChannelID, binding.AccountID, binding.Enabled, nil); err != nil {
			return err
		}
		return gorm.G[models.AgentGatewayBinding](tx).Create(ctx, binding)
	}); err != nil {
		slog.ErrorContext(ctx, "failed to create gateway binding", "error", err, "agentId", agentID)
		return nil, err
	}
	return binding.ToDto(), nil
}

func (s *GatewayService) UpdateBinding(ctx context.Context, agentID, bindingID uuid.UUID, dto *models.UpdateAgentGatewayBindingDto) (*models.AgentGatewayBindingDto, error) {
	binding, err := s.getBinding(ctx, agentID, bindingID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	defaultModelIDSet := dto.DefaultModelIDSet || dto.DefaultModelID != nil
	if defaultModelIDSet {
		if dto.DefaultModelID != nil {
			updates["default_model_id"] = *dto.DefaultModelID
		} else {
			updates["default_model_id"] = nil
		}
	}
	if dto.Enabled != nil {
		updates["enabled"] = *dto.Enabled
	}
	if len(updates) == 0 {
		return nil, customerrors.ErrNoChangesSpecified
	}

	enabled := binding.Enabled
	if dto.Enabled != nil {
		enabled = *dto.Enabled
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if defaultModelIDSet && dto.DefaultModelID != nil {
			if err := s.validateBindingModel(ctx, tx, agentID, *dto.DefaultModelID); err != nil {
				return err
			}
		}
		if err := s.ensureBindingUniqueness(ctx, tx, agentID, binding.ChannelID, binding.AccountID, enabled, &bindingID); err != nil {
			return err
		}
		return tx.WithContext(ctx).
			Model(&models.AgentGatewayBinding{}).
			Where("id = ? AND agent_id = ?", bindingID, agentID).
			Updates(updates).Error
	}); err != nil {
		slog.ErrorContext(ctx, "failed to update gateway binding", "error", err, "agentId", agentID, "bindingId", bindingID)
		return nil, err
	}
	return s.getBindingDto(ctx, agentID, bindingID)
}

func (s *GatewayService) reloadActiveGateway(ctx context.Context, tx *gorm.DB, reload GatewayReloadFunc) error {
	if reload == nil {
		return nil
	}
	channels, err := s.listEnabledChannelConfigs(ctx, tx)
	if err != nil {
		return err
	}
	return reload(ctx, channels)
}

func (s *GatewayService) DeleteBinding(ctx context.Context, agentID, bindingID uuid.UUID) error {
	if _, err := s.getBinding(ctx, agentID, bindingID); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("binding_id = ?", bindingID).Delete(&models.GatewayMessageDelivery{}).Error; err != nil {
			return err
		}
		if err := tx.Where("binding_id = ?", bindingID).Delete(&models.GatewayConversation{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND agent_id = ?", bindingID, agentID).Delete(&models.AgentGatewayBinding{}).Error
	}); err != nil {
		slog.ErrorContext(ctx, "failed to delete gateway binding", "error", err, "agentId", agentID, "bindingId", bindingID)
		return err
	}
	return nil
}

func (s *GatewayService) ResolveBinding(ctx context.Context, channelID, accountID string) (*models.AgentGatewayBinding, error) {
	binding, err := gorm.G[models.AgentGatewayBinding](s.db).
		Where("channel_id = ? AND account_id = ? AND enabled = TRUE", channelID, accountID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrGatewayBindingNotFound
		}
		return nil, err
	}
	return &binding, nil
}

func (s *GatewayService) GetConversation(ctx context.Context, channelID, accountID, conversationID, senderID string, agentID uuid.UUID) (*models.GatewayConversation, error) {
	conversation, err := gorm.G[models.GatewayConversation](s.db).
		Where("channel_id = ? AND account_id = ? AND conversation_id = ? AND sender_id = ? AND agent_id = ?", channelID, accountID, conversationID, senderID, agentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &conversation, nil
}

func (s *GatewayService) CreateConversation(ctx context.Context, conversation *models.GatewayConversation) error {
	return gorm.G[models.GatewayConversation](s.db).Create(ctx, conversation)
}

func (s *GatewayService) TouchConversation(ctx context.Context, conversationID uuid.UUID) error {
	return s.db.WithContext(ctx).
		Model(&models.GatewayConversation{}).
		Where("id = ?", conversationID).
		Update("updated_at", conn.NowExpr()).Error
}

func (s *GatewayService) GetSessionModelID(ctx context.Context, sessionID uuid.UUID) (uuid.UUID, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, customerrors.ErrSessionNotFound
		}
		return uuid.Nil, err
	}
	return session.LastUsedModel, nil
}

func (s *GatewayService) FindMessageDelivery(ctx context.Context, channelID, accountID, externalMessageID string) (*models.GatewayMessageDelivery, error) {
	delivery, err := gorm.G[models.GatewayMessageDelivery](s.db).
		Where("channel_id = ? AND account_id = ? AND external_message_id = ?", channelID, accountID, externalMessageID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &delivery, nil
}

func (s *GatewayService) CreateMessageDelivery(ctx context.Context, delivery *models.GatewayMessageDelivery) error {
	return gorm.G[models.GatewayMessageDelivery](s.db).Create(ctx, delivery)
}

func (s *GatewayService) FinishMessageDelivery(ctx context.Context, deliveryID uuid.UUID, status, errorText string) error {
	return s.db.WithContext(ctx).
		Model(&models.GatewayMessageDelivery{}).
		Where("id = ?", deliveryID).
		Updates(map[string]any{
			"status":     status,
			"error":      errorText,
			"updated_at": conn.NowExpr(),
		}).Error
}

func (s *GatewayService) HydrateAgentBindings(ctx context.Context, agents []*models.AgentDto) error {
	if len(agents) == 0 {
		return nil
	}
	agentIDs := make([]uuid.UUID, 0, len(agents))
	dtoMap := make(map[uuid.UUID]*models.AgentDto, len(agents))
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		agentIDs = append(agentIDs, agent.ID)
		dtoMap[agent.ID] = agent
	}
	if len(agentIDs) == 0 {
		return nil
	}
	bindings, err := gorm.G[models.AgentGatewayBinding](s.db).
		Where("agent_id IN ?", agentIDs).
		Order("created_at ASC").
		Find(ctx)
	if err != nil {
		return err
	}
	for i := range bindings {
		dto := bindings[i].ToDto()
		agent := dtoMap[dto.AgentID]
		if agent == nil {
			continue
		}
		agent.GatewayBindings = append(agent.GatewayBindings, *dto)
	}
	return nil
}

func (s *GatewayService) validateBindingInput(ctx context.Context, tx *gorm.DB, agentID uuid.UUID, channelID string, defaultModelID *uuid.UUID) (*models.GatewayChannel, error) {
	if err := s.ensureAgentExists(ctx, tx, agentID); err != nil {
		return nil, err
	}
	channel, err := s.getChannelModelWithDB(ctx, tx, channelID)
	if err != nil {
		return nil, err
	}
	if defaultModelID != nil {
		if err := s.validateBindingModel(ctx, tx, agentID, *defaultModelID); err != nil {
			return nil, err
		}
	}
	return channel, nil
}

func (s *GatewayService) ensureAgentExists(ctx context.Context, db *gorm.DB, agentID uuid.UUID) error {
	_, err := gorm.G[models.Agent](db).
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return customerrors.ErrAgentNotFound
		}
		return err
	}
	return nil
}

func (s *GatewayService) validateBindingModel(ctx context.Context, db *gorm.DB, agentID, modelID uuid.UUID) error {
	var count int64
	if err := db.WithContext(ctx).
		Model(&models.AgentModel{}).
		Joins("JOIN models ON models.id = agent_models.model_id").
		Where("agent_models.agent_id = ? AND agent_models.model_id = ? AND models.deleted_at IS NULL AND models.embedding_model IS FALSE", agentID, modelID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return customerrors.ErrGatewayBindingModelInvalid
	}
	return nil
}

func (s *GatewayService) ensureBindingUniqueness(ctx context.Context, tx *gorm.DB, agentID uuid.UUID, channelID, accountID string, enabled bool, excludeID *uuid.UUID) error {
	query := gorm.G[models.AgentGatewayBinding](tx).
		Where("agent_id = ? AND channel_id = ? AND account_id = ?", agentID, channelID, accountID)
	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}
	count, err := query.Count(ctx, "id")
	if err != nil {
		return err
	}
	if count > 0 {
		return customerrors.ErrGatewayBindingAlreadyExists
	}
	if !enabled {
		return nil
	}
	query = gorm.G[models.AgentGatewayBinding](tx).
		Where("channel_id = ? AND account_id = ? AND enabled = TRUE", channelID, accountID)
	if excludeID != nil {
		query = query.Where("id != ?", *excludeID)
	}
	count, err = query.Count(ctx, "id")
	if err != nil {
		return err
	}
	if count > 0 {
		return customerrors.ErrGatewayChannelConflict
	}
	return nil
}

func (s *GatewayService) updateBindingsForChannelIdentity(ctx context.Context, tx *gorm.DB, channelID string, channelType models.ChannelType, accountID string) error {
	var current []models.AgentGatewayBinding
	if err := tx.WithContext(ctx).
		Where("channel_id = ?", channelID).
		Find(&current).Error; err != nil {
		return err
	}
	if len(current) == 0 {
		return nil
	}
	currentIDs := make(map[uuid.UUID]struct{}, len(current))
	currentAgents := make(map[uuid.UUID]struct{}, len(current))
	enabledCurrent := false
	for _, binding := range current {
		currentIDs[binding.ID] = struct{}{}
		if _, ok := currentAgents[binding.AgentID]; ok {
			return customerrors.ErrGatewayBindingAlreadyExists
		}
		currentAgents[binding.AgentID] = struct{}{}
		if binding.Enabled {
			if enabledCurrent {
				return customerrors.ErrGatewayChannelConflict
			}
			enabledCurrent = true
		}
	}
	var existing []models.AgentGatewayBinding
	if err := tx.WithContext(ctx).
		Where("channel_id = ? AND account_id = ?", channelID, accountID).
		Find(&existing).Error; err != nil {
		return err
	}
	for _, other := range existing {
		if _, ok := currentIDs[other.ID]; ok {
			continue
		}
		if _, ok := currentAgents[other.AgentID]; ok {
			return customerrors.ErrGatewayBindingAlreadyExists
		}
		if enabledCurrent && other.Enabled {
			return customerrors.ErrGatewayChannelConflict
		}
	}
	if err := tx.WithContext(ctx).
		Model(&models.AgentGatewayBinding{}).
		Where("channel_id = ?", channelID).
		Updates(map[string]any{
			"channel_type": channelType,
			"account_id":   accountID,
			"updated_at":   conn.NowExpr(),
		}).Error; err != nil {
		if isDuplicateKeyError(err) {
			return customerrors.ErrGatewayChannelConflict
		}
		return err
	}
	return nil
}

func (s *GatewayService) getBinding(ctx context.Context, agentID, bindingID uuid.UUID) (*models.AgentGatewayBinding, error) {
	binding, err := gorm.G[models.AgentGatewayBinding](s.db).
		Where("id = ? AND agent_id = ?", bindingID, agentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrGatewayBindingNotFound
		}
		return nil, err
	}
	return &binding, nil
}

func (s *GatewayService) getBindingDto(ctx context.Context, agentID, bindingID uuid.UUID) (*models.AgentGatewayBindingDto, error) {
	binding, err := s.getBinding(ctx, agentID, bindingID)
	if err != nil {
		return nil, err
	}
	return binding.ToDto(), nil
}

func (s *GatewayService) getChannelModel(ctx context.Context, channelID string) (*models.GatewayChannel, error) {
	return s.getChannelModelWithDB(ctx, s.db, channelID)
}

func (s *GatewayService) getChannelModelWithDB(ctx context.Context, db *gorm.DB, channelID string) (*models.GatewayChannel, error) {
	channel := &models.GatewayChannel{}
	if err := db.WithContext(ctx).Where("id = ?", strings.TrimSpace(channelID)).First(channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrGatewayChannelNotFound
		}
		return nil, err
	}
	return channel, nil
}

func (s *GatewayService) applyChannelConfig(channel *models.GatewayChannel, discord *models.GatewayDiscordChannelConfig) error {
	switch channel.Type {
	case models.ChannelTypeDiscord:
		merged := discord
		if merged != nil && strings.TrimSpace(merged.BotToken) == "" {
			if existing := channel.DiscordConfig(); existing != nil {
				clone := *merged
				clone.BotToken = existing.BotToken
				merged = &clone
			}
		}
		return channel.SetDiscordConfig(merged)
	case models.ChannelTypeSlack, models.ChannelTypeTelegram, models.ChannelTypeWechat, models.ChannelTypeQQ:
		channel.Config = nil
		return nil
	default:
		return customerrors.ErrInvalidParams
	}
}

func (s *GatewayService) validateChannelModel(ctx context.Context, db *gorm.DB, channel *models.GatewayChannel, currentID string) error {
	channel.ID = strings.TrimSpace(channel.ID)
	channel.Type = models.ChannelType(strings.TrimSpace(string(channel.Type)))
	channel.AccountID = strings.TrimSpace(channel.AccountID)
	if channel.ID == "" || channel.Type == "" || channel.AccountID == "" {
		return customerrors.ErrInvalidParams
	}
	if channel.ID == "cli" {
		return customerrors.ErrInvalidParams
	}
	switch channel.Type {
	case models.ChannelTypeDiscord:
		if channel.Enabled {
			discord := channel.DiscordConfig()
			if discord == nil || strings.TrimSpace(discord.BotToken) == "" {
				return customerrors.ErrInvalidParams
			}
		}
	case models.ChannelTypeSlack, models.ChannelTypeTelegram, models.ChannelTypeWechat, models.ChannelTypeQQ:
	default:
		return customerrors.ErrInvalidParams
	}

	var count int64
	query := db.WithContext(ctx).Model(&models.GatewayChannel{}).
		Where("id = ?", channel.ID)
	if currentID != "" {
		query = query.Where("id != ?", currentID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return customerrors.ErrGatewayChannelAlreadyExists
	}

	if !channel.Enabled {
		return nil
	}
	query = db.WithContext(ctx).Model(&models.GatewayChannel{}).
		Where("type = ? AND account_id = ? AND enabled = TRUE", channel.Type, channel.AccountID)
	if currentID != "" {
		query = query.Where("id != ?", currentID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return customerrors.ErrGatewayChannelConflict
	}
	return nil
}

func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key")
}

func gatewayChannelCreateDuplicateError(err error) error {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "idx_gateway_channels_type_account_enabled") ||
		(strings.Contains(msg, "type") && strings.Contains(msg, "account")) {
		return customerrors.ErrGatewayChannelConflict
	}
	return customerrors.ErrGatewayChannelAlreadyExists
}
