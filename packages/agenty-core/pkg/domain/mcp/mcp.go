package mcp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Transport identifies the wire transport used by an MCP server.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
	TransportSSE   Transport = "sse"
)

func (transport Transport) Valid() bool {
	switch transport {
	case TransportStdio, TransportHTTP, TransportSSE:
		return true
	default:
		return false
	}
}

// OAuthConfig contains optional client registration settings. Access and
// refresh tokens are kept outside the server configuration file.
type OAuthConfig struct {
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Issuer       string `json:"issuer,omitempty"`
}

// Config is the concise, file-backed MCP server configuration. The file name
// supplies the server name, so it is intentionally absent from this object.
type Config struct {
	Type              Transport         `json:"type"`
	Enabled           bool              `json:"enabled"`
	Command           string            `json:"command,omitempty"`
	Args              []string          `json:"args,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	URL               string            `json:"url,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	BearerTokenEnvVar string            `json:"bearerTokenEnvVar,omitempty"`
	OAuth             *OAuthConfig      `json:"oauth,omitempty"`
}

func (config *Config) UnmarshalJSON(data []byte) error {
	type alias Config
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, present := fields["enabled"]; !present {
		decoded.Enabled = true
	}
	if _, present := fields["type"]; !present {
		switch {
		case decoded.Command != "":
			decoded.Type = TransportStdio
		case decoded.URL != "":
			decoded.Type = TransportHTTP
		}
	}
	*config = Config(decoded)
	return nil
}

func (config Config) Validate() error {
	if !config.Type.Valid() {
		return fmt.Errorf("mcp: unsupported transport %q", config.Type)
	}
	switch config.Type {
	case TransportStdio:
		if strings.TrimSpace(config.Command) == "" {
			return fmt.Errorf("mcp: stdio command is required")
		}
		if config.URL != "" {
			return fmt.Errorf("mcp: url is only valid for HTTP transports")
		}
		if len(config.Headers) > 0 || config.BearerTokenEnvVar != "" || config.OAuth != nil {
			return fmt.Errorf("mcp: headers, bearer token, and oauth are only valid for HTTP transports")
		}
	case TransportHTTP, TransportSSE:
		if strings.TrimSpace(config.URL) == "" {
			return fmt.Errorf("mcp: url is required for %s transport", config.Type)
		}
		parsed, err := url.Parse(config.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("mcp: url must be an absolute HTTP(S) URL")
		}
		if config.Command != "" || len(config.Args) > 0 || len(config.Env) > 0 {
			return fmt.Errorf("mcp: command and env are only valid for stdio transport")
		}
	}
	for key := range config.Env {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00\r\n") {
			return fmt.Errorf("mcp: invalid environment variable name %q", key)
		}
	}
	for key := range config.Headers {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\x00\r\n") {
			return fmt.Errorf("mcp: invalid header name %q", key)
		}
	}
	for key, value := range config.Headers {
		if strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("mcp: invalid header value for %q", key)
		}
	}
	if config.OAuth != nil {
		if config.OAuth.ClientID == "" && (config.OAuth.ClientSecret != "" || config.OAuth.Issuer != "") {
			return fmt.Errorf("mcp: oauth clientSecret and issuer require clientId")
		}
		for field, value := range map[string]string{
			"clientId": config.OAuth.ClientID, "clientSecret": config.OAuth.ClientSecret, "issuer": config.OAuth.Issuer,
		} {
			if strings.ContainsAny(value, "\x00\r\n") {
				return fmt.Errorf("mcp: invalid oauth %s", field)
			}
		}
	}
	if config.BearerTokenEnvVar != "" && (strings.TrimSpace(config.BearerTokenEnvVar) == "" || strings.ContainsAny(config.BearerTokenEnvVar, "=\x00\r\n")) {
		return fmt.Errorf("mcp: invalid bearer token environment variable name %q", config.BearerTokenEnvVar)
	}
	return nil
}

func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("mcp: server name is required")
	}

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("mcp: invalid server name %q", name)
	}
	return nil
}

type Status string

const (
	StatusDisabled       Status = "disabled"
	StatusConnecting     Status = "connecting"
	StatusConnected      Status = "connected"
	StatusAuthRequired   Status = "auth-required"
	StatusAuthenticating Status = "authenticating"
	StatusError          Status = "error"
	StatusClosing        Status = "closing"
)

// Server is the status projection returned to the local management client.
// Runtime OAuth tokens are kept in mcp-auth and are never part of this value.
type Server struct {
	Name        string `json:"name"`
	Config      Config `json:"config"`
	Status      Status `json:"status"`
	Error       string `json:"error,omitempty"`
	ErrorStage  string `json:"errorStage,omitempty"`
	ToolCount   int    `json:"toolCount"`
	LastChanged string `json:"lastChanged,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
	AuthURL     string `json:"authUrl,omitempty"`
}

// LogEntry is a bounded, in-memory diagnostic entry for one MCP server.
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Source  string `json:"source"`
	Stage   string `json:"stage,omitempty"`
	Message string `json:"message"`
}

type Event struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Status     Status `json:"status,omitempty"`
	Error      string `json:"error,omitempty"`
	ErrorStage string `json:"errorStage,omitempty"`
	ToolCount  int    `json:"toolCount,omitempty"`
	AuthURL    string `json:"authUrl,omitempty"`
}
