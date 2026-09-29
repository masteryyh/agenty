package instance

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	json "github.com/bytedance/sonic"
	"golang.org/x/crypto/sha3"
)

var ErrAlreadyRunning = errors.New("agenty core is already running for this data directory")

type Lock struct {
	file *os.File
}

func CanonicalDataDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("data directory is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve data directory: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", fmt.Errorf("create data directory: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("canonicalize data directory: %w", err)
	}
	canonical = filepath.Clean(canonical)
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	return canonical, nil
}

func RuntimeDir(dataDir string) string {
	return filepath.Join(dataDir, "runtime")
}

func AddressForDataDir(dataDir string) (transport, address string) {
	digest := sha3.Sum256([]byte(dataDir))
	key := hex.EncodeToString(digest[:16])
	if runtime.GOOS == "windows" {
		return "named_pipe", `\\.\pipe\agenty-core-` + key
	}
	return "uds", filepath.Join(os.TempDir(), "agenty-"+key+".sock")
}

type RuntimeMetadata struct {
	DataDir       string `json:"dataDir"`
	Transport     string `json:"transport"`
	Address       string `json:"address"`
	IPCVersion    string `json:"ipcVersion"`
	Protocol      string `json:"protocol"`
	ProcessID     int    `json:"processId"`
	EventStreamID string `json:"eventStreamId"`
}

func WriteMetadata(dataDir string, metadata RuntimeMetadata) (string, error) {
	runtimeDir := RuntimeDir(dataDir)
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return "", fmt.Errorf("create runtime directory: %w", err)
	}
	path := filepath.Join(runtimeDir, "core.json")
	temp, err := os.CreateTemp(runtimeDir, ".core-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create runtime metadata: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return "", err
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		temp.Close()
		return "", fmt.Errorf("encode runtime metadata: %w", err)
	}
	if _, err := temp.Write(append(encoded, '\n')); err != nil {
		temp.Close()
		return "", fmt.Errorf("write runtime metadata: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", fmt.Errorf("flush runtime metadata: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close runtime metadata: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return "", fmt.Errorf("install runtime metadata: %w", err)
	}
	return path, nil
}
