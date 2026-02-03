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

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type ConfigManager struct {
	cfg    *AppConfig
	vipers *viper.Viper
}

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		cfg:    &AppConfig{},
		vipers: viper.New(),
	}
}

func (cm *ConfigManager) GetConfig() *AppConfig {
	return cm.cfg
}

func (cm *ConfigManager) Validate() error {
	return cm.cfg.Validate()
}

func (cm *ConfigManager) BindEnvVariables() {
	cm.vipers.SetEnvPrefix("AGENTY")
	cm.vipers.AutomaticEnv()
	cm.vipers.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	envs := map[string]string{
		"port":        "AGENTY_PORT",
		"db.host":     "AGENTY_DB_HOST",
		"db.port":     "AGENTY_DB_PORT",
		"db.username": "AGENTY_DB_USERNAME",
		"db.password": "AGENTY_DB_PASSWORD",
		"db.database": "AGENTY_DB_DATABASE",
	}

	for key, env := range envs {
		cm.vipers.BindEnv(key, env)
	}
}

func (cm *ConfigManager) SetDefaults() {
	cm.vipers.SetDefault("port", 8080)
	cm.vipers.SetDefault("db.host", "localhost")
	cm.vipers.SetDefault("db.port", 5432)
	cm.vipers.SetDefault("db.username", "postgres")
	cm.vipers.SetDefault("db.database", "agenty")
}

func (cm *ConfigManager) LoadConfig(configPaths ...string) error {
	cm.SetDefaults()

	cm.vipers.SetConfigName("agenty")
	cm.vipers.SetConfigType("yaml")

	defaultPaths := []string{
		".",
		"./config",
		"./configs",
		"/etc/agenty",
		"$HOME/.agenty",
	}

	allPaths := append(configPaths, defaultPaths...)
	for _, path := range allPaths {
		cm.vipers.AddConfigPath(path)
	}

	if err := cm.vipers.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("warning: no config file found, using defaults")
		} else {
			return fmt.Errorf("error reading config file: %w", err)
		}
	} else {
		fmt.Printf("using config file: %s\n", cm.vipers.ConfigFileUsed())
	}

	if err := cm.mergeAdditionalConfigs(); err != nil {
		return err
	}

	if err := cm.vipers.Unmarshal(cm.cfg); err != nil {
		return fmt.Errorf("unable to decode config into struct: %w", err)
	}
	return nil
}

func (cm *ConfigManager) mergeAdditionalConfigs() error {
	configFile := cm.vipers.ConfigFileUsed()
	if configFile == "" {
		return nil
	}

	dir := filepath.Dir(configFile)

	fragments, err := cm.discoverFragments(configFile)
	if err != nil {
		return err
	}

	includeFragments, err := cm.resolveIncludes(dir)
	if err != nil {
		return err
	}

	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(fragments)+len(includeFragments))

	appendUnique := func(paths []string) {
		for _, p := range paths {
			clean := filepath.Clean(p)
			if clean == filepath.Clean(configFile) {
				continue
			}
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			ordered = append(ordered, clean)
		}
	}

	appendUnique(fragments)
	appendUnique(includeFragments)

	for _, fragment := range ordered {
		if err := cm.mergeConfigFile(fragment); err != nil {
			return err
		}
		fmt.Printf("merged config fragment: %s\n", fragment)
	}

	return nil
}

func (cm *ConfigManager) discoverFragments(configFile string) ([]string, error) {
	dir := filepath.Dir(configFile)
	base := strings.TrimSuffix(filepath.Base(configFile), filepath.Ext(configFile))

	patterns := []string{
		filepath.Join(dir, fmt.Sprintf("%s.*.yaml", base)),
		filepath.Join(dir, fmt.Sprintf("%s.*.yml", base)),
	}

	var matches []string
	for _, pattern := range patterns {
		globbed, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob pattern %q failed: %w", pattern, err)
		}
		if len(globbed) == 0 {
			continue
		}
		sort.Strings(globbed)
		matches = append(matches, globbed...)
	}

	return matches, nil
}

func (cm *ConfigManager) resolveIncludes(baseDir string) ([]string, error) {
	includes := cm.vipers.GetStringSlice("include")
	if len(includes) == 0 {
		return nil, nil
	}

	resolved := make([]string, 0, len(includes))
	for _, inc := range includes {
		if strings.TrimSpace(inc) == "" {
			continue
		}

		candidate := inc
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(baseDir, candidate)
		}

		info, err := os.Stat(candidate)
		if err != nil {
			return nil, fmt.Errorf("include file %q not accessible: %w", candidate, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("include path %q is a directory", candidate)
		}

		resolved = append(resolved, candidate)
	}

	return resolved, nil
}

func (cm *ConfigManager) mergeConfigFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config fragment %q: %w", path, err)
	}

	fragment := viper.New()

	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".yaml", ".yml":
		fragment.SetConfigType("yaml")
	case ".json":
		fragment.SetConfigType("json")
	case ".toml":
		fragment.SetConfigType("toml")
	default:
		return fmt.Errorf("unsupported config fragment type %q for file %s", ext, path)
	}

	if err := fragment.ReadConfig(bytes.NewReader(content)); err != nil {
		return fmt.Errorf("failed to parse config fragment %q: %w", path, err)
	}

	if err := cm.vipers.MergeConfigMap(fragment.AllSettings()); err != nil {
		return fmt.Errorf("failed to merge config fragment %q: %w", path, err)
	}

	return nil
}

var (
	globalConfigManager *ConfigManager
	once                sync.Once
)

func Init(files ...string) error {
	var err error
	once.Do(func() {
		globalConfigManager = NewConfigManager()
		globalConfigManager.BindEnvVariables()

		if err = globalConfigManager.LoadConfig(files...); err != nil {
			return
		}

		err = globalConfigManager.Validate()
	})
	return err
}

func GetConfigManager() *ConfigManager {
	return globalConfigManager
}
