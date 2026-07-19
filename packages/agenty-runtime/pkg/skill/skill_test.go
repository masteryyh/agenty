package skill

import (
	"strings"
	"testing"
)

func TestListBuiltinSkills(t *testing.T) {
	skills, err := ListBuiltinSkills()
	if err != nil {
		t.Fatalf("ListBuiltinSkills() error = %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("ListBuiltinSkills() returned no skills")
	}

	found := false
	for _, skill := range skills {
		if skill.Name != "agenty" {
			continue
		}
		found = true
		content := string(skill.Content)
		if !strings.Contains(content, "metadata:\n  id: 019decb1-850b-7efb-8261-966c19224492") {
			t.Fatal("builtin agenty skill is missing metadata id")
		}
		if !strings.Contains(content, "name: agenty") {
			t.Fatal("builtin agenty skill frontmatter name mismatch")
		}
	}

	if !found {
		t.Fatal("builtin agenty skill not found")
	}
}
