package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const appName = "sonicli"

// Config contains deliberately non-secret application settings.
type Config struct {
	ClientID string
	Theme    string
	Unicode  bool
	Player   string
}

func Default() Config {
	return Config{Theme: "default", Unicode: true, Player: "auto"}
}

func Dir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, appName), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	for lineNo, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return cfg, fmt.Errorf("config line %d: expected key = value", lineNo+1)
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch key {
		case "client_id":
			v, err := strconv.Unquote(value)
			if err != nil {
				return cfg, fmt.Errorf("config line %d: %w", lineNo+1, err)
			}
			cfg.ClientID = strings.TrimSpace(v)
		case "theme":
			v, err := strconv.Unquote(value)
			if err != nil {
				return cfg, fmt.Errorf("config line %d: %w", lineNo+1, err)
			}
			cfg.Theme = v
		case "unicode":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return cfg, fmt.Errorf("config line %d: %w", lineNo+1, err)
			}
			cfg.Unicode = v
		case "player":
			v, err := strconv.Unquote(value)
			if err != nil {
				return cfg, fmt.Errorf("config line %d: %w", lineNo+1, err)
			}
			cfg.Player = v
		}
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data := fmt.Sprintf("# Sonicli settings (OAuth tokens are stored separately).\nclient_id = %q\ntheme = %q\nunicode = %t\nplayer = %q\n", cfg.ClientID, cfg.Theme, cfg.Unicode, cfg.Player)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func DataDir() (string, error) {
	if root := os.Getenv("XDG_DATA_HOME"); root != "" {
		return filepath.Join(root, appName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", appName), nil
}
