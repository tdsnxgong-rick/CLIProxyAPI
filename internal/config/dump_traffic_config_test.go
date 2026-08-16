package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDumpTrafficConfig_DefaultsAndParsing(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
dump-traffic:
  enabled: true
  dir: "custom/traffic"
  raw-token: true
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.DumpTraffic.Enabled {
		t.Errorf("expected DumpTraffic.Enabled to be true")
	}
	if cfg.DumpTraffic.Dir != "custom/traffic" {
		t.Errorf("expected DumpTraffic.Dir to be %q, got %q", "custom/traffic", cfg.DumpTraffic.Dir)
	}
	if !cfg.DumpTraffic.RawToken {
		t.Errorf("expected DumpTraffic.RawToken to be true")
	}
}

func TestDumpTrafficConfig_DefaultDir(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")

	yamlContent := `
dump-traffic:
  enabled: true
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.DumpTraffic.Enabled {
		t.Errorf("expected DumpTraffic.Enabled to be true")
	}
	if cfg.DumpTraffic.Dir != "logs/traffic" {
		t.Errorf("expected DumpTraffic.Dir to default to %q, got %q", "logs/traffic", cfg.DumpTraffic.Dir)
	}
}
