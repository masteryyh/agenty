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

package command

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

func skillLabel(s models.SkillDto) string {
	var scopeBadge string
	if s.Scope == models.SkillScopeProject {
		scopeBadge = styleCyan.Render("[P]")
	} else {
		scopeBadge = styleGray.Render("[G]")
	}
	return fmt.Sprintf("%s %s", scopeBadge, s.Name)
}

func handleSkillCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	for {
		skills, err := b.ListSkills(sessionID)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list skills: %w", err)
		}

		if len(skills) == 0 {
			bridge.Info("No skills available")
			bridge.Info("  Global skills: %s", styleGray.Render("~/.agent/skills/"))
			bridge.Info("  Project skills: %s or %s", styleGray.Render("<cwd>/.agents/skills/"), styleGray.Render("<cwd>/.claude/skills/"))
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(skills))
		for i, s := range skills {
			desc := s.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			items[i] = fmt.Sprintf("%s  %s", skillLabel(s), styleGray.Render(desc))
		}

		res, err := bridge.ShowList("Skills", items, "↑/↓ navigate  ·  Enter view  ·  Esc back")
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		case ListActionSelect:
			if res.Index >= 0 && res.Index < len(skills) {
				selected := skills[res.Index]
				if err := showSkillContent(b, bridge, selected, sessionID); err != nil {
					bridge.Error("Failed to load skill: %v", err)
				}
			}
		}
	}
}

func showSkillContent(b backend.Backend, bridge Bridge, skill models.SkillDto, sessionID uuid.UUID) error {
	content, err := b.GetSkillContent(skill.Name, &sessionID)
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader(skill.Name))

	if skill.Scope != "" {
		var scopeStr string
		if skill.Scope == models.SkillScopeProject {
			scopeStr = styleCyan.Render("project")
		} else {
			scopeStr = styleGray.Render("global")
		}
		sb.WriteString(renderKV("Scope", scopeStr, 12))
	}
	sb.WriteString(renderKV("Path", styleGray.Render(skill.SkillMDPath), 12))
	sb.WriteString("\n")

	sb.WriteString(styleGray.Render(strings.Repeat("─", 60)))
	sb.WriteString("\n\n")
	sb.WriteString(content)
	sb.WriteString("\n")

	bridge.Print(sb.String())
	return nil
}
