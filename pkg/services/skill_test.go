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

package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestParseSkillMDMetadataID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "example")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	id := uuid.MustParse("019decb1-850b-7efb-8261-966c19224492")
	path := filepath.Join(dir, skillMDFileName)
	content := []byte(`---
name: example
description: Example skill
metadata:
  id: 019decb1-850b-7efb-8261-966c19224492
---

# Example
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	parsed, err := (&SkillService{}).parseSkillMD(path)
	if err != nil {
		t.Fatalf("parseSkillMD() error = %v", err)
	}
	if parsed.ID == nil || *parsed.ID != id {
		t.Fatalf("parseSkillMD() ID = %v, want %v", parsed.ID, id)
	}
}

func TestEnsureBuiltinSkillsWritesOnce(t *testing.T) {
	dir := t.TempDir()
	service := &SkillService{builtinSkillsPath: dir}

	if err := service.ensureBuiltinSkills(context.Background()); err != nil {
		t.Fatalf("ensureBuiltinSkills() error = %v", err)
	}

	path := filepath.Join(dir, "agenty", skillMDFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("builtin skill was not written: %v", err)
	}

	custom := []byte("custom")
	if err := os.WriteFile(path, custom, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := service.ensureBuiltinSkills(context.Background()); err != nil {
		t.Fatalf("ensureBuiltinSkills() second call error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(custom) {
		t.Fatal("ensureBuiltinSkills() overwrote existing builtin skill")
	}
}
