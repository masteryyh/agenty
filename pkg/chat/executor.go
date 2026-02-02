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

package chat

import (
	"context"
	"sync"

	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/openai/openai-go/v3"
)

type ChatExecutor struct {
	model  string
	client *openai.Client
}

func NewChatExecutor() *ChatExecutor {
	cfg := config.GetConfigManager().GetConfig()
	return &ChatExecutor{
		model:  cfg.Provider.OpenAI.Model,
		client: conn.GetOpenAIClient(cfg.Provider.OpenAI),
	}
}

var (
	chatExecutor *ChatExecutor
	once         sync.Once
)

func GetChatExecutor() *ChatExecutor {
	once.Do(func() {
		chatExecutor = NewChatExecutor()
	})
	return chatExecutor
}

func (ce *ChatExecutor) Chat(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (string, int64, error) {
	params := openai.ChatCompletionNewParams{
		Model:    ce.model,
		Messages: messages,
	}

	resp, err := ce.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", 0, err
	}
	return resp.Choices[0].Message.Content, resp.Usage.TotalTokens, nil
}
