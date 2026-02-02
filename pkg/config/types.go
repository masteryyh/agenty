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

package config

import "fmt"

// AppConfig is the config definition for this app
type AppConfig struct {
	// Port of the HTTP server
	Port int `mapstructure:"port"`

	// DB configuration
	DB *DatabaseConfig `mapstructure:"db"`

	// Provider for models
	Provider *ModelProviderConfig `mapstructure:"modelProvider"`
}

// DatabaseConfig is the config definition for database connection, only postgresql is supported for now
type DatabaseConfig struct {
	// Host of the database server
	Host string `mapstructure:"host"`

	// Port of the database server
	Port int `mapstructure:"port"`

	// Username for database authentication
	Username string `mapstructure:"username"`

	// Password for database authentication
	Password string `mapstructure:"password"`

	// Database name
	Database string `mapstructure:"database"`
}

func (c *DatabaseConfig) Validate() error {
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = 5432
	}
	if c.Username == "" {
		c.Username = "postgres"
	}
	if c.Password == "" {
		return fmt.Errorf("database password is required")
	}
	if c.Database == "" {
		c.Database = "agenty"
	}
	return nil
}

func (c *AppConfig) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}
	return c.Provider.Validate()
}

// ModelProviderConfig is the config definition for model providers
type ModelProviderConfig struct {
	// OpenAI compatible provider configuration
	OpenAI *OpenAIConfig `mapstructure:"openai"`
}

func (c *ModelProviderConfig) Validate() error {
	return c.OpenAI.Validate()
}

// OpenAIConfig stores OpenAI-compatible provider configuration
type OpenAIConfig struct {
	// BaseURL of the OpenAI API
	BaseURL string `mapstructure:"baseUrl"`

	// APIKey for authentication
	APIKey string `mapstructure:"apiKey"`

	// Model to chat with
	Model string `mapstructure:"model"`
}

func (c *OpenAIConfig) Validate() error {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.openai.com/v1"
	}
	if c.APIKey == "" {
		return fmt.Errorf("OpenAI API key is required")
	}
	if c.Model == "" {
		c.Model = "gpt-5.2"
	}
	return nil
}
