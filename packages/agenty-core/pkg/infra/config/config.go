package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/spf13/viper"
)

const (
	EnvDataDir   = "AGENTY_DATA_DIR"
	EnvLogLevel  = "AGENTY_LOG_LEVEL"
	EnvLogFormat = "AGENTY_LOG_FORMAT"
)

var (
	ErrConfigNotFound  = errors.New("config: no config file found")
	ErrHomeDirNotFound = errors.New("config: home dir cannot be resolved")
)

var configFiles = []struct {
	name   string
	format string
}{
	{"config.yaml", "yaml"},
	{"config.yml", "yaml"},
	{"config.json", "json"},
	{"config.toml", "toml"},
}

const defaultConfigFile = "config.json"

func Load() (*Config, *Paths, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, nil, err
	}

	file, format, err := findConfigFile(paths.DataDir)
	if err != nil {
		return nil, nil, err
	}
	if file == "" {
		return nil, nil, ErrConfigNotFound
	}

	v := viper.New()
	v.SetConfigType(format)
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	if err := v.ReadConfig(f); err != nil {
		return nil, nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, nil, err
	}
	applyEnvOverrides(&cfg, os.Getenv)

	paths.ConfigFile = file
	return &cfg, paths, nil
}

func applyEnvOverrides(cfg *Config, getenv func(string) string) {
	if v := getenv(EnvLogLevel); strings.TrimSpace(v) != "" {
		cfg.Logging.Level = v
	}

	if v := getenv(EnvLogFormat); strings.TrimSpace(v) != "" {
		cfg.Logging.Format = v
	}
}

func InitializeDataDir() error {
	paths, err := ResolvePaths()
	if err != nil {
		return err
	}

	for _, dir := range []string{
		paths.DataDir,
		paths.SessionsDir,
		paths.AgentsDir,
		paths.ProvidersDir,
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	if file, format, err := findConfigFile(paths.DataDir); err != nil {
		return err
	} else if file != "" {
		return validateConfigFile(file, format)
	}

	return writeDefaultConfig(paths.ConfigFile)
}

func validateConfigFile(file, format string) error {
	v := viper.New()
	v.SetConfigType(format)

	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := v.ReadConfig(f); err != nil {
		return errors.New("config: existing config file is malformed")
	}
	return nil
}

func writeDefaultConfig(file string) error {
	v := viper.New()
	v.Set("version", 1)
	v.Set("logging.level", "info")
	v.Set("logging.format", "text")

	data, err := json.MarshalIndent(v.AllSettings(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0644)
}

func findConfigFile(dataDir string) (file, format string, err error) {
	for _, cf := range configFiles {
		p := filepath.Join(dataDir, cf.name)
		if _, statErr := os.Stat(p); statErr == nil {
			return p, cf.format, nil
		} else if !os.IsNotExist(statErr) {
			return "", "", statErr
		}
	}
	return "", "", nil
}

func ResolvePaths() (*Paths, error) {
	dataDir := os.Getenv(EnvDataDir)
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, ErrHomeDirNotFound
		} else {
			dataDir = filepath.Join(home, ".agenty")
		}
	}

	return &Paths{
		DataDir:      dataDir,
		ConfigFile:   filepath.Join(dataDir, defaultConfigFile),
		SessionsDir:  filepath.Join(dataDir, "sessions"),
		AgentsDir:    filepath.Join(dataDir, "agents"),
		ProvidersDir: filepath.Join(dataDir, "providers"),
		DatabaseFile: filepath.Join(dataDir, "agenty.sqlite"),
	}, nil
}
