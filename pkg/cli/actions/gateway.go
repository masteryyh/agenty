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
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
)

func ListChannels(b backend.Backend, page, pageSize int) (*pagination.PagedResponse[models.GatewayChannelDto], error) {
	return b.ListChannels(page, pageSize)
}

func GetChannel(b backend.Backend, channelID string) (*models.GatewayChannelDto, error) {
	return b.GetChannel(strings.TrimSpace(channelID))
}

func CreateChannel(b backend.Backend, dto *models.CreateGatewayChannelDto) (*models.GatewayChannelDto, error) {
	return b.CreateChannel(dto)
}

func UpdateChannel(b backend.Backend, channelID string, dto *models.UpdateGatewayChannelDto) (*models.GatewayChannelDto, error) {
	return b.UpdateChannel(strings.TrimSpace(channelID), dto)
}

func DeleteChannel(b backend.Backend, channelID string) (*models.GatewayChannelDto, error) {
	channel, err := b.GetChannel(strings.TrimSpace(channelID))
	if err != nil {
		return nil, err
	}
	if err := b.DeleteChannel(channel.ID); err != nil {
		return nil, err
	}
	return channel, nil
}

func ListGatewayBindings(b backend.Backend, agentID *uuid.UUID) ([]models.AgentGatewayBindingDto, error) {
	return b.ListGatewayBindings(agentID)
}

func CreateGatewayBinding(b backend.Backend, agentID uuid.UUID, dto *models.CreateAgentGatewayBindingDto) (*models.AgentGatewayBindingDto, error) {
	dto.ChannelID = strings.TrimSpace(dto.ChannelID)
	return b.CreateGatewayBinding(agentID, dto)
}

func UpdateGatewayBinding(b backend.Backend, agentID, bindingID uuid.UUID, dto *models.UpdateAgentGatewayBindingDto) (*models.AgentGatewayBindingDto, error) {
	return b.UpdateGatewayBinding(agentID, bindingID, dto)
}

func DeleteGatewayBinding(b backend.Backend, agentID, bindingID uuid.UUID) error {
	return b.DeleteGatewayBinding(agentID, bindingID)
}
