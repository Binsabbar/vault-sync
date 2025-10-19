package core

import (
	"context"
	"os"
	"sync"
	"vault-sync/internal/config"
	repo "vault-sync/internal/repository"
	psqlRepo "vault-sync/internal/repository/postgres"
	"vault-sync/internal/service/orchestrator"
	"vault-sync/internal/service/pathmatching"
	"vault-sync/internal/vault"
	"vault-sync/pkg/db"
	"vault-sync/pkg/db/migrations"
	"vault-sync/pkg/log"

	"github.com/rs/zerolog"
)

type Wiring struct {
	config *config.Config
	logger zerolog.Logger
}

func NewWiring(cfg *config.Config) *Wiring {
	var once sync.Once
	var instance *Wiring
	once.Do(func() {
		instance = &Wiring{
			config: cfg,
			logger: log.Logger.With().Str("component", "wiring").Logger(),
		}
	})
	return instance
}

func (w *Wiring) InitPostgresDataStore() *db.PostgresDatastore {
	var instance *db.PostgresDatastore
	var once sync.Once
	once.Do(func() {
		var err error
		instance, err = db.NewPostgresDatastore(&w.config.Postgres, migrations.NewPostgresMigration())
		if err != nil {
			w.logger.Error().Err(err).Msg("Failed to create Postgres datastore")
			os.Exit(-1)
		}
	})
	return instance
}

func (w *Wiring) GetConfig() *config.Config {
	return w.config
}

func (w *Wiring) InitSyncedSecretRepository() repo.SyncedSecretRepository {
	return psqlRepo.NewSyncedSecretRepository(w.InitPostgresDataStore())
}

func (w *Wiring) InitVaultClient(ctx context.Context) vault.Syncer {
	configAsPointers := make([]*config.VaultClusterConfig, len(w.config.Vault.ReplicaClusters))
	for i := range w.config.Vault.ReplicaClusters {
		configAsPointers[i] = &w.config.Vault.ReplicaClusters[i]
	}

	var instance vault.Syncer
	var once sync.Once
	once.Do(func() {
		var err error
		instance, err = vault.NewMultiClusterVaultClient(ctx, &w.config.Vault.MainCluster, configAsPointers)
		if err != nil {
			w.logger.Error().Err(err).Msg("Failed to create Vault client")
			os.Exit(-1)
		}
	})

	return instance
}

func (w *Wiring) InitPathMatcher() *pathmatching.VaultPathMatcher {
	vaultClient := w.InitVaultClient(context.Background())
	return pathmatching.NewVaultPathMatcher(vaultClient, &w.config.SyncRule)
}

func (w *Wiring) InitOrchestrator(ctx context.Context) *orchestrator.SyncOrchestrator {
	vaultClient := w.InitVaultClient(ctx)
	dbClient := w.InitSyncedSecretRepository()
	pathMatcher := w.InitPathMatcher()

	return orchestrator.NewSyncOrchestrator(
		vaultClient,
		dbClient,
		pathMatcher,
		w.config.Concurrency,
	)
}
