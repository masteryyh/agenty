package agentloop

import (
	"fmt"
	"strings"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	domainskill "github.com/masteryyh/agenty-core/pkg/domain/skill"
)

func (engine *Engine) resolveExplicitSkills(content conversation.Content) ([]domainskill.Resolved, error) {
	var text strings.Builder
	for _, block := range content {
		if textBlock, ok := block.(conversation.TextBlock); ok {
			text.WriteString(textBlock.Text)
		}
	}
	if !strings.Contains(text.String(), "[$") {
		return nil, nil
	}
	if engine.skills == nil {
		return nil, fmt.Errorf("skill registry is unavailable")
	}
	return engine.skills.ResolveReferences(text.String())
}

func formatSkillMarkdown(skills []domainskill.Resolved) string {
	var builder strings.Builder
	for index, resolved := range skills {
		if index > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("<skill-md>\n")
		builder.WriteString(strings.TrimSuffix(resolved.Content, "\n"))
		builder.WriteString("\n</skill-md>")
	}
	return builder.String()
}

func skillNames(skills []domainskill.Resolved) []string {
	names := make([]string, 0, len(skills))
	for _, resolved := range skills {
		names = append(names, resolved.Entry.Name)
	}
	return names
}

func skillPaths(skills []domainskill.Resolved) []string {
	paths := make([]string, 0, len(skills))
	for _, resolved := range skills {
		paths = append(paths, resolved.Entry.Location)
	}
	return paths
}
