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

package actions

import (
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
)

func ListMCPServers(b backend.Backend, page, pageSize int) (*pagination.PagedResponse[models.MCPServerDto], error) {
	return b.ListMCPServers(page, pageSize)
}

func GetMCPServer(b backend.Backend, nameOrID string) (*models.MCPServerDto, error) {
	server, err := resolveMCPServer(b, nameOrID)
	if err != nil {
		return nil, err
	}
	return &server, nil
}

func CreateMCPServer(b backend.Backend, dto *models.CreateMCPServerDto) (*models.MCPServerDto, error) {
	return b.CreateMCPServer(dto)
}

func UpdateMCPServer(b backend.Backend, nameOrID string, dto *models.UpdateMCPServerDto) (*models.MCPServerDto, error) {
	server, err := resolveMCPServer(b, nameOrID)
	if err != nil {
		return nil, err
	}
	return b.UpdateMCPServer(server.ID, dto)
}

func DeleteMCPServer(b backend.Backend, nameOrID string) (*models.MCPServerDto, error) {
	server, err := resolveMCPServer(b, nameOrID)
	if err != nil {
		return nil, err
	}
	if err := b.DeleteMCPServer(server.ID); err != nil {
		return nil, err
	}
	return &server, nil
}

func ConnectMCPServer(b backend.Backend, nameOrID string) (*models.MCPServerDto, error) {
	server, err := resolveMCPServer(b, nameOrID)
	if err != nil {
		return nil, err
	}
	if err := b.ConnectMCPServer(server.ID); err != nil {
		return nil, err
	}
	return GetMCPServer(b, server.ID.String())
}

func DisconnectMCPServer(b backend.Backend, nameOrID string) (*models.MCPServerDto, error) {
	server, err := resolveMCPServer(b, nameOrID)
	if err != nil {
		return nil, err
	}
	if err := b.DisconnectMCPServer(server.ID); err != nil {
		return nil, err
	}
	return GetMCPServer(b, server.ID.String())
}

func resolveMCPServer(b backend.Backend, nameOrID string) (models.MCPServerDto, error) {
	servers, err := listAllPages(func(page, pageSize int) (*pagination.PagedResponse[models.MCPServerDto], error) {
		return b.ListMCPServers(page, pageSize)
	})
	if err != nil {
		return models.MCPServerDto{}, err
	}
	return resolveByNameOrID(nameOrID, servers, func(item models.MCPServerDto) uuid.UUID {
		return item.ID
	}, func(item models.MCPServerDto) string {
		return item.Name
	})
}
