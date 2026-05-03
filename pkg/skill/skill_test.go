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
