package sync

import (
	"fmt"

	"github.com/Binsabbar/vault-sync/internal/config"
	"github.com/Binsabbar/vault-sync/internal/vault"
)

// VaultClient defines the interface for interacting with Vault
type VaultClient interface {
	ListSecrets(path string) ([]string, error)
	ReadSecret(path string) (*vault.Secret, error)
	WriteSecret(secret *vault.Secret) error
}

// Syncer handles synchronization between Vault instances
type Syncer struct {
	sourceClient VaultClient
	targetClient VaultClient
}

// New creates a new Syncer instance
func New(cfg *config.Config) (*Syncer, error) {
	sourceClient, err := vault.NewClient(cfg.Source.Address, cfg.Source.Token, cfg.Source.Prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create source client: %w", err)
	}

	targetClient, err := vault.NewClient(cfg.Target.Address, cfg.Target.Token, cfg.Target.Prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create target client: %w", err)
	}

	return &Syncer{
		sourceClient: sourceClient,
		targetClient: targetClient,
	}, nil
}

// NewWithClients creates a new Syncer with provided clients (useful for testing)
func NewWithClients(sourceClient, targetClient VaultClient) *Syncer {
	return &Syncer{
		sourceClient: sourceClient,
		targetClient: targetClient,
	}
}

// Sync synchronizes secrets from source to target
func (s *Syncer) Sync(dryRun bool) error {
	return s.SyncPath("", dryRun)
}

// SyncPath synchronizes secrets from a specific path
func (s *Syncer) SyncPath(path string, dryRun bool) error {
	secrets, err := s.sourceClient.ListSecrets(path)
	if err != nil {
		return fmt.Errorf("failed to list source secrets at path '%s': %w", path, err)
	}

	for _, secretPath := range secrets {
		fullPath := s.buildSecretPath(path, secretPath)
		if err := s.syncSecret(fullPath, dryRun); err != nil {
			return fmt.Errorf("failed to sync secret '%s': %w", fullPath, err)
		}
	}

	return nil
}

// SyncSecret synchronizes a single secret (exported for testing)
func (s *Syncer) SyncSecret(path string, dryRun bool) error {
	return s.syncSecret(path, dryRun)
}

func (s *Syncer) syncSecret(path string, dryRun bool) error {
	secret, err := s.sourceClient.ReadSecret(path)
	if err != nil {
		return fmt.Errorf("failed to read secret from source: %w", err)
	}

	if dryRun {
		fmt.Printf("Would sync secret: %s\n", path)
		return nil
	}

	if err := s.targetClient.WriteSecret(secret); err != nil {
		return fmt.Errorf("failed to write secret to target: %w", err)
	}

	fmt.Printf("Synced secret: %s\n", path)
	return nil
}

func (s *Syncer) buildSecretPath(basePath, secretPath string) string {
	if basePath == "" {
		return secretPath
	}
	return basePath + "/" + secretPath
}