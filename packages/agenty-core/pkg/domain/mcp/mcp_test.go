package mcp

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestConfigDefaultsEnabledWhenOmitted(t *testing.T) {
	var config Config
	if err := json.Unmarshal([]byte(`{"url":"https://example.com/mcp"}`), &config); err != nil {
		t.Fatal(err)
	}
	if !config.Enabled {
		t.Fatal("enabled = false, want true when omitted")
	}
	if config.Type != TransportHTTP {
		t.Fatalf("type = %q, want %q when omitted", config.Type, TransportHTTP)
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestConfigValidationRejectsMixedTransportFields(t *testing.T) {
	config := Config{Type: TransportStdio, Enabled: true, Command: "server", Headers: map[string]string{"Authorization": "Bearer token"}}
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted HTTP fields on a stdio server")
	}

	config = Config{Type: TransportHTTP, Enabled: true, URL: "localhost:8080/mcp"}
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted a non-absolute HTTP URL")
	}

	config = Config{Type: TransportHTTP, Enabled: true, URL: "https://example.com/mcp", OAuth: &OAuthConfig{ClientSecret: "secret"}}
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted OAuth credentials without a client id")
	}
}

func TestConfigArgumentsUseAStringArray(t *testing.T) {
	original := Config{
		Type:    TransportStdio,
		Enabled: true,
		Command: "node",
		Args:    []string{"server.js", "--label", "value with spaces"},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !slices.Equal(decoded.Args, original.Args) {
		t.Fatalf("decoded args = %#v, want %#v", decoded.Args, original.Args)
	}

	var legacy Config
	if err := json.Unmarshal([]byte(`{"type":"stdio","command":"node","args":"--label value"}`), &legacy); err == nil {
		t.Fatal("Unmarshal accepted string args; MCP arguments must be a JSON array")
	}
}

func TestValidateName(t *testing.T) {
	for _, name := range []string{"github", "GitHub", "browser-use", "my_server_v2"} {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", ".hidden", "my_server.v2", "中文", "../escape", "has space", "a/b"} {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) accepted an invalid name", name)
		}
	}
}
