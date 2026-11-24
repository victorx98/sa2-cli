package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultOrigin     = "https://sa2.mentorx.org"
	DevelopmentOrigin = "http://localhost:8094"
)

type Settings struct {
	Origin string `json:"SA2_API_URL"`
}

func Load() (Settings, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Settings{}, fmt.Errorf("Cannot locate the user configuration directory: %w", err)
	}
	path := filepath.Join(dir, "sa2-cli", "config.json")
	cfg, err := load(path)
	if err != nil {
		return Settings{}, fmt.Errorf("Cannot load configuration %s: %w", path, err)
	}
	if origin := os.Getenv("SA2_API_URL"); origin != "" {
		cfg.Origin = origin
	}
	return cfg, nil
}

func load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := createDefault(path); err != nil {
			return Settings{}, err
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return Settings{}, err
	}

	var cfg Settings
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Settings{}, errors.New("expected a JSON object with string configuration values")
	}
	if cfg.Origin == "" {
		return Settings{}, errors.New("SA2_API_URL is required")
	}
	return cfg, nil
}

func createDefault(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(Settings{Origin: DefaultOrigin})
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}

	// Publish the complete file without replacing an existing configuration.
	if err := os.Link(file.Name(), path); !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}
