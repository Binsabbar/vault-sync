package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the configuration for vault-sync
type Config struct {
	Source VaultConfig `json:"source"`
	Target VaultConfig `json:"target"`
}

// VaultConfig represents configuration for a Vault instance
type VaultConfig struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	Prefix  string `json:"prefix"`
}

// Load loads configuration from file or command line arguments
func Load(configPath, source, target, token string) (*Config, error) {
	var cfg *Config
	
	if configPath != "" {
		fileConfig, err := loadFromFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
		cfg = fileConfig
	} else {
		cfg = &Config{}
	}

	// Override with command line arguments if provided
	if source != "" {
		cfg.Source.Address = source
	}
	if target != "" {
		cfg.Target.Address = target
	}
	if token != "" {
		cfg.Source.Token = token
		cfg.Target.Token = token
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func loadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) validate() error {
	if c.Source.Address == "" {
		return fmt.Errorf("source vault address is required")
	}
	if c.Target.Address == "" {
		return fmt.Errorf("target vault address is required")
	}
	if c.Source.Token == "" {
		return fmt.Errorf("source vault token is required")
	}
	if c.Target.Token == "" {
		return fmt.Errorf("target vault token is required")
	}
	return nil
}