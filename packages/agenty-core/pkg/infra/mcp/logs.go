package mcp

import (
	"strings"
	"time"
	"unicode"

	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
)

const maxServerLogMessageRunes = 2_000

func (registry *Registry) appendServerLog(
	name string,
	generation uint64,
	level string,
	source string,
	stage string,
	message string,
) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	entry, ok := registry.servers[serverKey(name)]
	if !ok || entry.generation != generation || registry.closing {
		return
	}
	registry.appendLogLocked(entry, generation, level, source, stage, message)
}

func (registry *Registry) appendLogLocked(
	entry *serverEntry,
	generation uint64,
	level string,
	source string,
	stage string,
	message string,
) {
	if entry == nil || entry.generation != generation {
		return
	}

	message = sanitizeMCPLog(message, entry.config)
	if message == "" {
		return
	}

	entry.logs = append(entry.logs, domainmcp.LogEntry{
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
		Level:   level,
		Source:  source,
		Stage:   stage,
		Message: message,
	})
	if len(entry.logs) > maxServerLogEntries {
		entry.logs = entry.logs[len(entry.logs)-maxServerLogEntries:]
	}
}

func sanitizeMCPLog(message string, config domainmcp.Config) string {
	value := strings.TrimSpace(message)
	if value == "" {
		return ""
	}

	for _, secret := range mcpLogSecrets(config) {
		value = strings.ReplaceAll(value, secret, "[REDACTED]")
	}

	var builder strings.Builder
	for _, character := range value {
		if character == '\t' {
			builder.WriteByte(' ')
			continue
		}
		if unicode.IsControl(character) {
			continue
		}
		builder.WriteRune(character)
	}

	runes := []rune(strings.TrimSpace(builder.String()))
	if len(runes) > maxServerLogMessageRunes {
		runes = runes[:maxServerLogMessageRunes]
	}
	return strings.TrimSpace(string(runes))
}

func mcpLogSecrets(config domainmcp.Config) []string {
	secrets := make([]string, 0)
	for _, value := range expandMap(config.Env) {
		if value != "" {
			secrets = append(secrets, value)
		}
	}
	for _, value := range expandMap(config.Headers) {
		if value != "" {
			secrets = append(secrets, value)
		}
	}
	return secrets
}

type stdioLogWriter struct {
	registry   *Registry
	name       string
	generation uint64
}

func (writer *stdioLogWriter) Write(data []byte) (int, error) {
	for _, line := range strings.Split(string(data), "\n") {
		writer.registry.appendServerLog(
			writer.name,
			writer.generation,
			"info",
			"server",
			"stderr",
			line,
		)
	}
	return len(data), nil
}
