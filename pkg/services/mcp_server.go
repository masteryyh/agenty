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

package services

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type MCPServerService struct {
	db *gorm.DB
}

var (
	mcpServerService *MCPServerService
	mcpServerOnce    sync.Once
)

func GetMCPServerService() *MCPServerService {
	mcpServerOnce.Do(func() {
		mcpServerService = &MCPServerService{
			db: conn.GetDB(),
		}
	})
	return mcpServerService
}

func (s *MCPServerService) CreateMCPServer(ctx context.Context, dto *models.CreateMCPServerDto) (*models.MCPServerDto, error) {
	exists, err := gorm.G[models.MCPServer](s.db).
		Where("name = ? AND deleted_at IS NULL", dto.Name).
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to check mcp server existence", "error", err)
		return nil, err
	}
	if exists > 0 {
		return nil, customerrors.ErrMCPServerAlreadyExists
	}

	server := &models.MCPServer{
		Name:      dto.Name,
		Transport: dto.Transport,
		Enabled:   true,
	}

	if dto.Enabled != nil {
		server.Enabled = *dto.Enabled
	}

	switch dto.Transport {
	case models.MCPTransportStdio:
		server.Command = dto.Command
		if len(dto.Args) > 0 {
			data, err := json.Marshal(dto.Args)
			if err != nil {
				slog.ErrorContext(ctx, "failed to marshal mcp server args", "error", err)
				return nil, err
			}
			server.Args = data
		}
		if len(dto.Env) > 0 {
			data, err := json.Marshal(dto.Env)
			if err != nil {
				slog.ErrorContext(ctx, "failed to marshal mcp server env", "error", err)
				return nil, err
			}
			server.Env = data
		}
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		server.URL = dto.URL
		if len(dto.Headers) > 0 {
			data, err := json.Marshal(dto.Headers)
			if err != nil {
				slog.ErrorContext(ctx, "failed to marshal mcp server headers", "error", err)
				return nil, err
			}
			server.Headers = data
		}
	}

	if err := gorm.G[models.MCPServer](s.db).Create(ctx, server); err != nil {
		slog.ErrorContext(ctx, "failed to create mcp server", "error", err)
		return nil, err
	}

	return server.ToDto(), nil
}

func (s *MCPServerService) GetMCPServer(ctx context.Context, serverID uuid.UUID) (*models.MCPServerDto, error) {
	server, err := gorm.G[models.MCPServer](s.db).
		Where("id = ? AND deleted_at IS NULL", serverID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrMCPServerNotFound
		}
		slog.ErrorContext(ctx, "failed to find mcp server", "error", err, "server_id", serverID)
		return nil, err
	}

	return server.ToDto(), nil
}

func (s *MCPServerService) GetMCPServerModel(ctx context.Context, serverID uuid.UUID) (*models.MCPServer, error) {
	server, err := gorm.G[models.MCPServer](s.db).
		Where("id = ? AND deleted_at IS NULL", serverID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrMCPServerNotFound
		}
		slog.ErrorContext(ctx, "failed to find mcp server", "error", err, "server_id", serverID)
		return nil, err
	}
	return &server, nil
}

func (s *MCPServerService) ListMCPServers(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.MCPServerDto], error) {
	servers, err := gorm.G[models.MCPServer](s.db).
		Where("deleted_at IS NULL").
		Offset((request.Page - 1) * request.PageSize).
		Limit(request.PageSize).
		Order("created_at DESC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list mcp servers", "error", err)
		return nil, err
	}

	total, err := gorm.G[models.MCPServer](s.db).
		Where("deleted_at IS NULL").
		Count(ctx, "id")
	if err != nil {
		slog.ErrorContext(ctx, "failed to count mcp servers", "error", err)
		return nil, err
	}

	dtos := lo.Map(servers, func(s models.MCPServer, _ int) models.MCPServerDto {
		return *s.ToDto()
	})

	return &pagination.PagedResponse[models.MCPServerDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *MCPServerService) ListAllEnabled(ctx context.Context) ([]models.MCPServer, error) {
	servers, err := gorm.G[models.MCPServer](s.db).
		Where("enabled = true AND deleted_at IS NULL").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list enabled mcp servers", "error", err)
		return nil, err
	}
	return servers, nil
}

func (s *MCPServerService) UpdateMCPServer(ctx context.Context, serverID uuid.UUID, dto *models.UpdateMCPServerDto) (*models.MCPServerDto, error) {
	server, err := gorm.G[models.MCPServer](s.db).
		Where("id = ? AND deleted_at IS NULL", serverID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrMCPServerNotFound
		}
		slog.ErrorContext(ctx, "failed to find mcp server", "error", err, "server_id", serverID)
		return nil, err
	}

	updates := make(map[string]any)

	if dto.Name != "" && server.Name != dto.Name {
		exists, err := gorm.G[models.MCPServer](s.db).
			Where("name = ? AND id != ? AND deleted_at IS NULL", dto.Name, serverID).
			Count(ctx, "id")
		if err != nil {
			return nil, err
		}
		if exists > 0 {
			return nil, customerrors.ErrMCPServerAlreadyExists
		}
		updates["name"] = dto.Name
	}

	effectiveTransport := server.Transport
	if dto.Transport != "" {
		effectiveTransport = dto.Transport
	}

	switch effectiveTransport {
	case models.MCPTransportStdio:
		effectiveCommand := server.Command
		if dto.Command != "" {
			effectiveCommand = dto.Command
		}
		if effectiveCommand == "" {
			return nil, customerrors.ErrInvalidParams
		}
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		effectiveURL := server.URL
		if dto.URL != "" {
			effectiveURL = dto.URL
		}
		if effectiveURL == "" {
			return nil, customerrors.ErrInvalidParams
		}
	}

	updates["transport"] = effectiveTransport
	if dto.Enabled != nil {
		updates["enabled"] = *dto.Enabled
	}

	switch effectiveTransport {
	case models.MCPTransportStdio:
		if dto.Command != "" {
			updates["command"] = dto.Command
		}
		if dto.Args != nil {
			if data, err := json.Marshal(dto.Args); err == nil {
				updates["args"] = data
			}
		}
		if dto.Env != nil {
			if data, err := json.Marshal(dto.Env); err == nil {
				updates["env"] = data
			}
		}
		updates["url"] = ""
		updates["headers"] = nil
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		if dto.URL != "" {
			updates["url"] = dto.URL
		}
		if dto.Headers != nil {
			if data, err := json.Marshal(dto.Headers); err == nil {
				updates["headers"] = data
			}
		}
		updates["command"] = ""
		updates["args"] = nil
		updates["env"] = nil
	}

	if err := s.db.WithContext(ctx).
		Model(&models.MCPServer{}).
		Where("id = ? AND deleted_at IS NULL", serverID).
		Updates(updates).Error; err != nil {
		slog.ErrorContext(ctx, "failed to update mcp server", "error", err, "server_id", serverID)
		return nil, err
	}

	return server.ToDto(), nil
}

func (s *MCPServerService) DeleteMCPServer(ctx context.Context, serverID uuid.UUID) error {
	_, err := gorm.G[models.MCPServer](s.db).
		Where("id = ? AND deleted_at IS NULL", serverID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return customerrors.ErrMCPServerNotFound
		}
		slog.ErrorContext(ctx, "failed to find mcp server", "error", err, "server_id", serverID)
		return err
	}

	if _, err := gorm.G[models.MCPServer](s.db).
		Where("id = ? AND deleted_at IS NULL", serverID).
		Update(ctx, "deleted_at", gorm.Expr("NOW()")); err != nil {
		slog.ErrorContext(ctx, "failed to delete mcp server", "error", err, "server_id", serverID)
		return err
	}

	return nil
}
