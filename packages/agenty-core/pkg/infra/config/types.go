package config

// Config is the parsed configuration.
type Config struct {
	// Version is the schema version of this config file. Reserved for future use.
	Version int `mapstructure:"version"`
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
