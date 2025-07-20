package sync

import (
	"fmt"

	"github.com/Binsabbar/vault-sync/internal/config"
	"github.com/Binsabbar/vault-sync/internal/vault"
)

// Syncer handles synchronization between Vault instances
type Syncer struct {
	sourceClient *vault.Client
	targetClient *vault.Client
}

// New creates a new Syncer instance
func New(cfg *config.Config) *Syncer {
	sourceClient, err := vault.NewClient(cfg.Source.Address, cfg.Source.Token, cfg.Source.Prefix)
	if err != nil {
		panic(fmt.Sprintf("failed to create source client: %v", err))
	}

	targetClient, err := vault.NewClient(cfg.Target.Address, cfg.Target.Token, cfg.Target.Prefix)
	if err != nil {
		panic(fmt.Sprintf("failed to create target client: %v", err))
	}

	return &Syncer{
		sourceClient: sourceClient,
		targetClient: targetClient,
	}
}

// Sync synchronizes secrets from source to target
func (s *Syncer) Sync(dryRun bool) error {
	secrets, err := s.sourceClient.ListSecrets("")
	if err != nil {
		return fmt.Errorf("failed to list source secrets: %w", err)
	}

	for _, secretPath := range secrets {
		if err := s.syncSecret(secretPath, dryRun); err != nil {
			return fmt.Errorf("failed to sync secret %s: %w", secretPath, err)
		}
	}

	return nil
}

func (s *Syncer) syncSecret(path string, dryRun bool) error {
	secret, err := s.sourceClient.ReadSecret(path)
	if err != nil {
		return fmt.Errorf("failed to read secret: %w", err)
	}

	if dryRun {
		fmt.Printf("Would sync secret: %s\n", path)
		return nil
	}

	if err := s.targetClient.WriteSecret(secret); err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}

	fmt.Printf("Synced secret: %s\n", path)
	return nil
}