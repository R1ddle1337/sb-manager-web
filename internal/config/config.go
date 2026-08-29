package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TLSConfig struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

type AgentConfig struct {
	Enabled           bool   `json:"enabled"`
	ControllerURL     string `json:"controller_url"`
	EnrollmentToken   string `json:"enrollment_token,omitempty"`
	IdentityFile      string `json:"identity_file"`
	HeartbeatInterval string `json:"heartbeat_interval"`
}

type TaskConfig struct {
	DefaultTimeout time.Duration `json:"-"`
	TimeoutText    string        `json:"default_timeout"`
	Concurrency    int           `json:"batch_concurrency"`
	FailureStopPct int           `json:"failure_stop_percent"`
}

type Config struct {
	Listen       string      `json:"listen"`
	SBPath       string      `json:"sb_path"`
	DataDir      string      `json:"data_dir"`
	Database     string      `json:"database"`
	LogDir       string      `json:"log_dir"`
	HelperSocket string      `json:"helper_socket"`
	TLS          TLSConfig   `json:"tls"`
	Agent        AgentConfig `json:"agent"`
	Tasks        TaskConfig  `json:"tasks"`
}

func Defaults() Config {
	return Config{
		Listen:       "127.0.0.1:9091",
		SBPath:       "/usr/local/bin/sb",
		DataDir:      "/var/lib/sb-manager-web",
		Database:     "/var/lib/sb-manager-web/web.db",
		LogDir:       "/var/log/sb-manager-web",
		HelperSocket: "/run/sb-manager-web/helper.sock",
		Agent: AgentConfig{
			IdentityFile:      "/var/lib/sb-manager-web/agent-identity/ed25519.key",
			HeartbeatInterval: "30s",
		},
		Tasks: TaskConfig{TimeoutText: "10m", Concurrency: 1, FailureStopPct: 25},
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if path == "" {
		path = "/etc/sb-manager-web/config.json"
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg.Tasks.DefaultTimeout = 10 * time.Minute
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.Tasks.DefaultTimeout = 10 * time.Minute
	if cfg.Tasks.TimeoutText != "" {
		parsed, err := time.ParseDuration(cfg.Tasks.TimeoutText)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("invalid tasks.default_timeout: %q", cfg.Tasks.TimeoutText)
		}
		cfg.Tasks.DefaultTimeout = parsed
	}
	if cfg.Listen == "" || cfg.SBPath == "" || cfg.DataDir == "" || cfg.Database == "" {
		return Config{}, errors.New("listen, sb_path, data_dir and database are required")
	}
	if cfg.Tasks.Concurrency < 1 || cfg.Tasks.Concurrency > 32 {
		return Config{}, errors.New("tasks.batch_concurrency must be between 1 and 32")
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if path == "" {
		return errors.New("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
