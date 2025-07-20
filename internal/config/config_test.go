package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_WithConfigFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	
	configData := Config{
		Source: VaultConfig{
			Address: "http://source.vault:8200",
			Token:   "source-token",
			Prefix:  "secret",
		},
		Target: VaultConfig{
			Address: "http://target.vault:8200",
			Token:   "target-token",
			Prefix:  "secret",
		},
	}
	
	data, err := json.Marshal(configData)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	// Test loading the config
	cfg, err := Load(configPath, "", "", "")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	
	if cfg.Source.Address != "http://source.vault:8200" {
		t.Errorf("Expected source address 'http://source.vault:8200', got '%s'", cfg.Source.Address)
	}
	
	if cfg.Target.Address != "http://target.vault:8200" {
		t.Errorf("Expected target address 'http://target.vault:8200', got '%s'", cfg.Target.Address)
	}
}

func TestLoad_WithCommandLineArgs(t *testing.T) {
	cfg, err := Load("", "http://source:8200", "http://target:8200", "test-token")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	
	if cfg.Source.Address != "http://source:8200" {
		t.Errorf("Expected source address 'http://source:8200', got '%s'", cfg.Source.Address)
	}
	
	if cfg.Target.Address != "http://target:8200" {
		t.Errorf("Expected target address 'http://target:8200', got '%s'", cfg.Target.Address)
	}
	
	if cfg.Source.Token != "test-token" {
		t.Errorf("Expected source token 'test-token', got '%s'", cfg.Source.Token)
	}
	
	if cfg.Target.Token != "test-token" {
		t.Errorf("Expected target token 'test-token', got '%s'", cfg.Target.Token)
	}
}

func TestLoad_OverrideConfigFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	
	configData := Config{
		Source: VaultConfig{
			Address: "http://file-source:8200",
			Token:   "file-token",
		},
		Target: VaultConfig{
			Address: "http://file-target:8200",
			Token:   "file-token",
		},
	}
	
	data, err := json.Marshal(configData)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
	
	// Test loading the config with overrides
	cfg, err := Load(configPath, "http://cli-source:8200", "", "cli-token")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	
	if cfg.Source.Address != "http://cli-source:8200" {
		t.Errorf("Expected source address 'http://cli-source:8200', got '%s'", cfg.Source.Address)
	}
	
	if cfg.Target.Address != "http://file-target:8200" {
		t.Errorf("Expected target address 'http://file-target:8200', got '%s'", cfg.Target.Address)
	}
	
	if cfg.Source.Token != "cli-token" {
		t.Errorf("Expected source token 'cli-token', got '%s'", cfg.Source.Token)
	}
}

func TestValidate_MissingFields(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		expect string
	}{
		{
			name:   "missing source address",
			config: Config{Target: VaultConfig{Address: "addr", Token: "token"}},
			expect: "source vault address is required",
		},
		{
			name:   "missing target address",
			config: Config{Source: VaultConfig{Address: "addr", Token: "token"}},
			expect: "target vault address is required",
		},
		{
			name: "missing source token",
			config: Config{
				Source: VaultConfig{Address: "addr"},
				Target: VaultConfig{Address: "addr", Token: "token"},
			},
			expect: "source vault token is required",
		},
		{
			name: "missing target token",
			config: Config{
				Source: VaultConfig{Address: "addr", Token: "token"},
				Target: VaultConfig{Address: "addr"},
			},
			expect: "target vault token is required",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if err == nil {
				t.Fatalf("Expected validation error, got nil")
			}
			if err.Error() != tt.expect {
				t.Errorf("Expected error '%s', got '%s'", tt.expect, err.Error())
			}
		})
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	config := Config{
		Source: VaultConfig{Address: "http://source:8200", Token: "source-token"},
		Target: VaultConfig{Address: "http://target:8200", Token: "target-token"},
	}
	
	if err := config.validate(); err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}