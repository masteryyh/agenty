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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type CLIConfig struct {
	BaseURL  string `mapstructure:"baseUrl"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

var cliConfig *CLIConfig

func loadCLIConfig() error {
	v := viper.New()
	v.SetConfigName("cli-config")
	v.SetConfigType("yaml")

	v.SetDefault("baseUrl", "http://localhost:8080")

	homeDir, err := os.UserHomeDir()
	if err == nil {
		v.AddConfigPath(filepath.Join(homeDir, ".agenty"))
	}
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			cliConfig = &CLIConfig{
				BaseURL: "http://localhost:8080",
			}
			return nil
		}
		return fmt.Errorf("error reading CLI config file: %w", err)
	}

	cliConfig = &CLIConfig{}
	if err := v.Unmarshal(cliConfig); err != nil {
		return fmt.Errorf("unable to decode CLI config: %w", err)
	}

	return nil
}

func GetCLIConfig() *CLIConfig {
	if cliConfig == nil {
		if err := loadCLIConfig(); err != nil {
			cliConfig = &CLIConfig{
				BaseURL: "http://localhost:8080",
			}
		}
	}
	return cliConfig
}
