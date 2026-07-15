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

package skill

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const BuiltinSkillsDirName = "builtin-skills"

//go:embed *.md
var files embed.FS

type BuiltinSkill struct {
	Name    string
	Content []byte
}

func BuiltinDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil || homeDir == "" {
			if err != nil {
				return "", fmt.Errorf("failed to get user config directory: %w", err)
			}
			return "", fmt.Errorf("failed to get user home directory: %w", homeErr)
		}
		configDir = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(configDir, "agenty", BuiltinSkillsDirName), nil
}

func ListBuiltinSkills() ([]BuiltinSkill, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("failed to read builtin skills: %w", err)
	}

	skills := make([]BuiltinSkill, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		content, err := files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read builtin skill %s: %w", entry.Name(), err)
		}

		skills = append(skills, BuiltinSkill{
			Name:    strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
			Content: content,
		})
	}
	return skills, nil
}
