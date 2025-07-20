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
	cfg, err := loadConfiguration(configPath)
	if err != nil {
		return nil, err
	}

	applyCommandLineOverrides(cfg, source, target, token)

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func loadConfiguration(configPath string) (*Config, error) {
	if configPath != "" {
		return loadFromFile(configPath)
	}
	return &Config{}, nil
}

func applyCommandLineOverrides(cfg *Config, source, target, token string) {
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
}

func loadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file '%s': %w", path, err)
	}

	return &config, nil
}

func (c *Config) validate() error {
	if err := c.validateVaultConfig("source", c.Source); err != nil {
		return err
	}
	if err := c.validateVaultConfig("target", c.Target); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateVaultConfig(name string, cfg VaultConfig) error {
	if cfg.Address == "" {
		return fmt.Errorf("%s vault address is required", name)
	}
	if cfg.Token == "" {
		return fmt.Errorf("%s vault token is required", name)
	}
	return nil
}