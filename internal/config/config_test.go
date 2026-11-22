package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestLoadCreatesAndReloadsConfiguration(t *testing.T) {
	dir := t.TempDir()
	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", dir)
		dir = filepath.Join(dir, "Library", "Application Support")
	case "windows":
		t.Setenv("APPDATA", dir)
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	t.Setenv("SA2_API_URL", "")
	t.Setenv("FEISHU_APP_ID", "")

	var callers sync.WaitGroup
	const wantOrigin = "https://sa2.mentorx.org"
	for range 8 {
		callers.Go(func() {
			if cfg, err := Load(); err != nil || cfg.Origin != wantOrigin {
				t.Errorf("Load() origin = %q, %v; want %q", cfg.Origin, err, wantOrigin)
			}
		})
	}
	callers.Wait()

	path := filepath.Join(dir, "sa2-cli", "config.json")
	edited := []byte(`{"SA2_API_URL":"https://sa2.example.test","FEISHU_APP_ID":"cli_example"}`)
	if err := os.WriteFile(path, edited, 0600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(); err != nil || cfg.Origin != "https://sa2.example.test" {
		t.Fatalf("Load() = %+v, %v; want the edited settings", cfg, err)
	}

	t.Setenv("SA2_API_URL", "https://override.example.test")
	t.Setenv("FEISHU_APP_ID", "cli_override")
	if cfg, err := Load(); err != nil || cfg.Origin != "https://override.example.test" {
		t.Fatalf("Load() = %+v, %v; want the environment overrides", cfg, err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || string(saved) != string(edited) {
		t.Fatalf("configuration changed with environment override: %q, %v", saved, err)
	}

	if err := os.WriteFile(path, []byte(`{"SA2_API_URL":}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted malformed configuration")
	}
}
