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

package conn

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"
)

func GetOpenAIClient(baseUrl, apiKey string) *openai.Client {
	client := openai.NewClient(
		openaiOption.WithBaseURL(baseUrl),
		openaiOption.WithAPIKey(apiKey),
	)
	return &client
}

func GetAnthropicClient(baseURL, apiKey string) anthropic.Client {
	opts := []anthropicOption.RequestOption{anthropicOption.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, anthropicOption.WithBaseURL(baseURL))
	}
	return anthropic.NewClient(opts...)
}

func GetGeminiClient(ctx context.Context, baseURL, apiKey string) (*genai.Client, error) {
	cc := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}
	if baseURL != "" {
		cc.HTTPOptions = genai.HTTPOptions{BaseURL: baseURL}
	}

	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return client, nil
}
