package skill

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	domainskill "github.com/masteryyh/agenty-core/pkg/domain/skill"
)

type Registry struct {
	entries      map[string]domainskill.Entry
	seenDirs     map[string]string
	diagnostics  []domainskill.Diagnostic
	orderedNames []string
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type root struct {
	path   string
	source string
}

func Scan(dataSkillsDir string) (*Registry, error) {
	if strings.TrimSpace(dataSkillsDir) == "" {
		return nil, errors.New("skill data directory must not be empty")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory for skills: %w", err)
	}
	return ScanRoots([]struct {
		Path   string
		Source string
	}{
		{Path: dataSkillsDir, Source: "agenty"},
		{Path: filepath.Join(home, ".agents", "skills"), Source: "agents"},
		{Path: filepath.Join(home, ".claude", "skills"), Source: "claude"},
	})
}

func ScanRoots(roots []struct {
	Path   string
	Source string
}) (*Registry, error) {
	registry := &Registry{
		entries:  make(map[string]domainskill.Entry),
		seenDirs: make(map[string]string),
	}
	for _, candidateRoot := range roots {
		rootPath, err := filepath.Abs(candidateRoot.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve skill root %q: %w", candidateRoot.Path, err)
		}
		if err := registry.scanRoot(root{path: rootPath, source: candidateRoot.Source}); err != nil {
			return nil, err
		}
	}

	registry.orderedNames = make([]string, 0, len(registry.entries))
	for name := range registry.entries {
		registry.orderedNames = append(registry.orderedNames, name)
	}
	sort.Strings(registry.orderedNames)
	return registry, nil
}

func (registry *Registry) scanRoot(candidate root) error {
	entries, err := os.ReadDir(candidate.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		registry.diagnostics = append(registry.diagnostics, domainskill.Diagnostic{
			Severity: "warning",
			Code:     "skill_root_read_failed",
			Message:  fmt.Sprintf("read skill directory %q: %v", candidate.path, err),
			Path:     candidate.path,
		})
		return nil
	}

	for _, directory := range entries {
		if !directory.IsDir() {
			if directory.Type()&os.ModeSymlink == 0 {
				continue
			}

			info, statErr := os.Stat(filepath.Join(candidate.path, directory.Name()))
			if statErr != nil || !info.IsDir() {
				continue
			}
		}

		if directory.Name() == "." || directory.Name() == ".." {
			continue
		}

		directoryPath := filepath.Join(candidate.path, directory.Name())
		if _, seen := registry.seenDirs[directory.Name()]; seen {
			continue
		}
		registry.seenDirs[directory.Name()] = directoryPath

		location := filepath.Join(directoryPath, "SKILL.md")
		if _, err := os.Stat(location); err != nil {
			if !os.IsNotExist(err) {
				registry.diagnostics = append(registry.diagnostics, domainskill.Diagnostic{
					Severity: "warning",
					Code:     "skill_file_stat_failed",
					Message:  fmt.Sprintf("stat %q: %v", location, err),
					Path:     location,
				})
			}
			continue
		}

		data, err := os.ReadFile(location)
		if err != nil {
			registry.diagnostics = append(registry.diagnostics, domainskill.Diagnostic{
				Severity: "warning",
				Code:     "skill_file_read_failed",
				Message:  fmt.Sprintf("read %q: %v", location, err),
				Path:     location,
			})
			continue
		}

		meta, err := parseFrontmatter(data)
		if err != nil {
			registry.diagnostics = append(registry.diagnostics, domainskill.Diagnostic{
				Severity: "warning",
				Code:     "skill_frontmatter_invalid",
				Message:  fmt.Sprintf("parse %q: %v", location, err),
				Path:     location,
			})
			continue
		}
		if meta.Name == "" || meta.Description == "" {
			registry.diagnostics = append(registry.diagnostics, domainskill.Diagnostic{
				Severity: "warning",
				Code:     "skill_metadata_missing",
				Message:  fmt.Sprintf("skill %q must define non-empty name and description", location),
				Path:     location,
			})
			continue
		}

		entry := domainskill.Entry{
			Name:          meta.Name,
			DirectoryName: directory.Name(),
			Description:   strings.TrimSpace(meta.Description),
			Location:      location,
			Source:        candidate.source,
			AutoEnabled:   meta.Name == directory.Name(),
		}
		if !entry.AutoEnabled {
			entry.Warning = fmt.Sprintf("frontmatter name %q does not match directory %q", meta.Name, directory.Name())
			registry.diagnostics = append(registry.diagnostics, domainskill.Diagnostic{
				Severity: "warning",
				Code:     "skill_name_mismatch",
				Message:  entry.Warning,
				Path:     location,
				Name:     meta.Name,
			})
		}
		if _, exists := registry.entries[entry.Name]; exists {
			continue
		}
		registry.entries[entry.Name] = entry
	}
	return nil
}

func parseFrontmatter(data []byte) (frontmatter, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return frontmatter{}, errors.New("SKILL.md must start with YAML frontmatter")
	}

	var builder strings.Builder
	foundEnd := false
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "---" {
			foundEnd = true
			break
		}
		builder.WriteString(scanner.Text())
		builder.WriteByte('\n')
	}
	if !foundEnd {
		return frontmatter{}, errors.New("YAML frontmatter closing marker is missing")
	}

	var result frontmatter
	if err := yaml.Unmarshal([]byte(builder.String()), &result); err != nil {
		return frontmatter{}, err
	}
	result.Name = strings.TrimSpace(result.Name)
	result.Description = strings.TrimSpace(result.Description)
	return result, nil
}

func (registry *Registry) Entries() []domainskill.Entry {
	entries := make([]domainskill.Entry, 0, len(registry.orderedNames))
	for _, name := range registry.orderedNames {
		entries = append(entries, registry.entries[name])
	}
	return entries
}

func (registry *Registry) Diagnostics() []domainskill.Diagnostic {
	if len(registry.diagnostics) == 0 {
		return []domainskill.Diagnostic{}
	}
	return append([]domainskill.Diagnostic(nil), registry.diagnostics...)
}

func (registry *Registry) Get(name string) (domainskill.Entry, bool) {
	entry, ok := registry.entries[name]
	return entry, ok
}

func (registry *Registry) AutomaticEntries() []domainskill.Entry {
	entries := make([]domainskill.Entry, 0)
	for _, name := range registry.orderedNames {
		entry := registry.entries[name]
		if entry.AutoEnabled {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (registry *Registry) PromptSection() string {
	entries := registry.AutomaticEntries()
	if len(entries) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("## Skills\n\n")
	builder.WriteString("Skills provide specialized instructions for particular tasks. Evaluate the available skill names and descriptions for every user request. When a skill is relevant, use read_file to read its complete SKILL.md at the listed location before taking related actions. If multiple skills apply, use the smallest set that covers the task and apply them in a clear order. Resolve relative paths against the skill directory. User instructions and higher-priority instructions take precedence.\n\n")
	builder.WriteString("<skills>\n")
	for _, entry := range entries {
		builder.WriteString("  <skill>\n")
		builder.WriteString("    <name>")
		builder.WriteString(xmlEscape(entry.Name))
		builder.WriteString("</name>\n    <description>")
		builder.WriteString(xmlEscape(entry.Description))
		builder.WriteString("</description>\n    <location>")
		builder.WriteString(xmlEscape(entry.Location))
		builder.WriteString("</location>\n  </skill>\n")
	}
	builder.WriteString("</skills>")
	return builder.String()
}

func xmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	return strings.ReplaceAll(value, "'", "&apos;")
}

func ParseReferences(text string) []domainskill.Reference {
	var references []domainskill.Reference
	seen := make(map[string]struct{})
	inFence := false
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}

		if inFence {
			continue
		}

		for offset := 0; offset < len(line); {
			start := strings.Index(line[offset:], "[$")
			if start < 0 {
				break
			}
			start += offset
			if insideInlineCode(line, start) {
				offset = start + 2
				continue
			}
			nameEnd := strings.IndexByte(line[start+2:], ']')
			if nameEnd < 0 {
				break
			}
			nameEnd += start + 2
			if nameEnd+1 >= len(line) || line[nameEnd+1] != '(' {
				offset = nameEnd + 1
				continue
			}
			pathStart := nameEnd + 2
			pathEnd := pathStart
			escaped := false
			for pathEnd < len(line) {
				if line[pathEnd] == ')' && !escaped {
					break
				}
				if line[pathEnd] == '\\' && !escaped {
					escaped = true
				} else {
					escaped = false
				}
				pathEnd++
			}
			if pathEnd >= len(line) {
				break
			}
			name := line[start+2 : nameEnd]
			path := unescapeMarkdownPath(line[pathStart:pathEnd])
			key := name + "\x00" + path
			if name != "" && path != "" {
				if _, exists := seen[key]; !exists {
					seen[key] = struct{}{}
					references = append(references, domainskill.Reference{Name: name, Path: path})
				}
			}
			offset = pathEnd + 1
		}
	}
	return references
}

func unescapeMarkdownPath(path string) string {
	var builder strings.Builder
	escaped := false
	for _, character := range path {
		if escaped {
			if character == ')' || character == '\\' {
				builder.WriteRune(character)
			} else {
				builder.WriteByte('\\')
				builder.WriteRune(character)
			}
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		builder.WriteRune(character)
	}
	if escaped {
		builder.WriteByte('\\')
	}
	return builder.String()
}

func insideInlineCode(line string, offset int) bool {
	backticks := 0
	for index := range offset {
		if line[index] == '`' && (index == 0 || line[index-1] != '\\') {
			backticks++
		}
	}
	return backticks%2 == 1
}

func (registry *Registry) ResolveReferences(text string) ([]domainskill.Resolved, error) {
	var resolved []domainskill.Resolved
	for _, reference := range ParseReferences(text) {
		entry, ok := registry.Get(reference.Name)
		if !ok {
			return nil, fmt.Errorf("skill %q is not available", reference.Name)
		}
		cleanPath, err := filepath.Abs(reference.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve skill %q path: %w", reference.Name, err)
		}
		if err := entry.ValidateReference(reference.Name, cleanPath); err != nil {
			return nil, err
		}
		content, err := os.ReadFile(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("read skill %q: %w", reference.Name, err)
		}
		current, err := parseFrontmatter(content)
		if err != nil {
			return nil, fmt.Errorf("parse skill %q after discovery: %w", reference.Name, err)
		}
		if current.Name != entry.Name {
			return nil, fmt.Errorf("skill %q changed its frontmatter name to %q; restart Agenty to rescan", entry.Name, current.Name)
		}
		resolved = append(resolved, domainskill.Resolved{Entry: entry, Content: string(content)})
	}
	return resolved, nil
}
