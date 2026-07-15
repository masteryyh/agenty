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
	"path/filepath"
	"testing"
)

func TestNewServerConfigDefaults(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg, err := NewServerConfig(0, "", false)
	if err != nil {
		t.Fatalf("NewServerConfig() error = %v", err)
	}
	if cfg.Port != DefaultServerPort {
		t.Fatalf("Port = %d, want %d", cfg.Port, DefaultServerPort)
	}
	wantPath := filepath.Join(homeDir, ".agenty", "agenty.db")
	if cfg.DB.SQLitePath != wantPath {
		t.Fatalf("SQLitePath = %q, want %q", cfg.DB.SQLitePath, wantPath)
	}
	if cfg.Debug {
		t.Fatal("Debug = true, want false")
	}
}

func TestNewServerConfigRejectsInvalidPort(t *testing.T) {
	_, err := NewServerConfig(65536, filepath.Join(t.TempDir(), "agenty.db"), false)
	if err == nil {
		t.Fatal("NewServerConfig() error = nil, want invalid port error")
	}
}

func TestResolveSQLitePathRejectsNamedHome(t *testing.T) {
	if _, err := ResolveSQLitePath("~someone/agenty.db"); err == nil {
		t.Fatal("ResolveSQLitePath() error = nil, want unsupported home path error")
	}
}
