package logging

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/infra/config"
)

func TestLoadSettings(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	tests := []struct {
		name       string
		logging    config.LoggingConfig
		wantLevel  slog.Level
		wantFormat outputFormat
		wantFile   string
	}{
		{
			name:       "defaults",
			logging:    config.LoggingConfig{},
			wantLevel:  slog.LevelInfo,
			wantFormat: formatText,
			wantFile:   "core.log",
		},
		{
			name: "debug jsonl is case insensitive",
			logging: config.LoggingConfig{
				Level:  " DEBUG ",
				Format: "JSONL",
			},
			wantLevel:  slog.LevelDebug,
			wantFormat: formatJSONL,
			wantFile:   "core.jsonl",
		},
		{
			name: "warn text",
			logging: config.LoggingConfig{
				Level:  "warn",
				Format: "text",
			},
			wantLevel:  slog.LevelWarn,
			wantFormat: formatText,
			wantFile:   "core.log",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings, err := loadSettings("/data", test.logging, now)
			if err != nil {
				t.Fatalf("loadSettings: %v", err)
			}
			if settings.level != test.wantLevel {
				t.Errorf("level = %s, want %s", settings.level, test.wantLevel)
			}
			if settings.format != test.wantFormat {
				t.Errorf("format = %d, want %d", settings.format, test.wantFormat)
			}
			wantPath := filepath.Join("/data", "logs", "2026", "07", "23", test.wantFile)
			if settings.path != wantPath {
				t.Errorf("path = %q, want %q", settings.path, wantPath)
			}
		})
	}
}

func TestLoadSettingsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		logging config.LoggingConfig
		wantErr string
	}{
		{
			name:    "invalid level",
			logging: config.LoggingConfig{Level: "trace"},
			wantErr: `invalid AGENTY_LOG_LEVEL "trace"`,
		},
		{
			name:    "invalid format",
			logging: config.LoggingConfig{Format: "json"},
			wantErr: `invalid AGENTY_LOG_FORMAT "json"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadSettings(t.TempDir(), test.logging, time.Now())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("loadSettings error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestOpenTextFiltersByLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "2026", "07", "23", "core.log")
	logger, err := open(settings{
		level:  slog.LevelInfo,
		format: formatText,
		path:   path,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	logger.Debug("hidden debug message")
	logger.Info("core ready", "component", "rpc")
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "hidden debug message") {
		t.Fatalf("log contains filtered debug entry: %q", text)
	}
	if !strings.Contains(text, `level=INFO msg="core ready" component=rpc`) {
		t.Fatalf("text log = %q", text)
	}
}

func TestOpenJSONLWritesOneObjectPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "2026", "07", "23", "core.jsonl")
	logger, err := open(settings{
		level:  slog.LevelDebug,
		format: formatJSONL,
		path:   path,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	logger.Debug("first", "requestId", "req-1")
	logger.Error("second", "error", "boom")
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	entries := make([]map[string]any, 0, 2)
	for scanner.Scan() {
		var entry map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode JSONL line %q: %v", scanner.Bytes(), err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan log: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0]["level"] != "DEBUG" || entries[0]["msg"] != "first" || entries[0]["requestId"] != "req-1" {
		t.Errorf("first entry = %#v", entries[0])
	}
	if entries[1]["level"] != "ERROR" || entries[1]["msg"] != "second" || entries[1]["error"] != "boom" {
		t.Errorf("second entry = %#v", entries[1])
	}
}

func TestOpenUsesConfiguredDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", dataDir)
	t.Setenv(config.EnvLogLevel, "error")
	t.Setenv(config.EnvLogFormat, "jsonl")
	config.ResetForTesting()

	logger, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	logger.Error("configured logger")
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	wantPrefix := filepath.Join(dataDir, "logs") + string(os.PathSeparator)
	if !strings.HasPrefix(logger.path, wantPrefix) || filepath.Base(logger.path) != "core.jsonl" {
		t.Fatalf("log path = %q, want under %q with core.jsonl", logger.path, wantPrefix)
	}
	if _, err := os.Stat(logger.path); err != nil {
		t.Fatalf("stat log: %v", err)
	}
}

func TestOpenReadsLoggingFromConfigFile(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", dataDir)
	t.Setenv(config.EnvLogLevel, "")
	t.Setenv(config.EnvLogFormat, "")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(dataDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{"version":1,"logging":{"level":"info","format":"jsonl"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	config.ResetForTesting()

	logger, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	logger.Info("from config file")
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	if filepath.Base(logger.path) != "core.jsonl" {
		t.Fatalf("log path = %q, want core.jsonl driven by config file", logger.path)
	}
}

func TestOpenEnvOverridesConfigFile(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", dataDir)
	t.Setenv(config.EnvLogLevel, "")
	t.Setenv(config.EnvLogFormat, "jsonl")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(dataDir, "config.json")
	// Config file says text; env says jsonl. Env must win.
	if err := os.WriteFile(configFile, []byte(`{"version":1,"logging":{"level":"info","format":"text"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	config.ResetForTesting()

	logger, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	logger.Info("env override")
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	if filepath.Base(logger.path) != "core.jsonl" {
		t.Fatalf("log path = %q, want core.jsonl (env overrode config file text)", logger.path)
	}
}
