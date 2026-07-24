package config

// Config is the parsed configuration. Values come from the config file first,
// then non-empty environment overrides are applied on top.
type Config struct {
	// Version is the schema version of this config file. Reserved for future use.
	Version int `mapstructure:"version"`

	// Logging configures the slog file logger. Empty fields fall back to the
	// logger defaults (info level, text format).
	Logging LoggingConfig `mapstructure:"logging"`
}

// LoggingConfig mirrors the AGENTY_LOG_LEVEL / AGENTY_LOG_FORMAT environment
// variables so the same settings can be expressed via config file. Environment
// values take precedence over file values when set.
type LoggingConfig struct {
	// Level accepts debug, info, warn, or error (case-insensitive).
	Level string `mapstructure:"level"`

	// Format accepts text or jsonl (case-insensitive).
	Format string `mapstructure:"format"`
}

// Paths holds the resolved filesystem locations derived from the data directory.
type Paths struct {
	// DataDir is the root: ~/.agenty by default, or $AGENTY_DATA_DIR if set.
	DataDir string

	// ConfigFile is the default config path (config.json) for creation, or the
	// actually loaded file when returned by Load.
	ConfigFile string

	// SessionsDir is DataDir/sessions, where JSONL transcripts live.
	SessionsDir string

	// AgentsDir is DataDir/agents, where agent JSON files live.
	AgentsDir string

	// ProvidersDir is DataDir/providers, where provider directories live.
	ProvidersDir string

	// DatabaseFile is DataDir/agenty.sqlite.
	DatabaseFile string
}
