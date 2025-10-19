package orchestrator

import (
	"context"
	"errors"
	"time"

	// "errors"
	"os"
	"testing"

	// "time"

	"github.com/stretchr/testify/suite"

	"vault-sync/internal/config"
	"vault-sync/internal/repository/postgres"
	"vault-sync/internal/service/pathmatching"
	"vault-sync/internal/vault"
	"vault-sync/pkg/db"
	"vault-sync/pkg/db/migrations"
	"vault-sync/testutil"
)

type OrchestratorTestSuite struct {
	suite.Suite
	ctx context.Context

	// Vault clusters
	vaultClient         *vault.MultiClusterVaultClient
	vaultMainHelper     *testutil.VaultHelper
	vaultReplica1Helper *testutil.VaultHelper
	vaultReplica2Helper *testutil.VaultHelper

	// Database
	pgHelper *testutil.PostgresHelper
	repo     *postgres.SyncedSecretRepository
}

const (
	teamAMount = "team-a"
	teamBMount = "team-b"
)

var (
	mounts = []string{teamAMount, teamBMount}
)

func (suite *OrchestratorTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.setupVaultClient()
	suite.setupRepositoryClient()
}

func (suite *OrchestratorTestSuite) setupVaultClient() {
	var err error
	result := testutil.SetupOneMainTwoReplicaClusters(mounts...)
	suite.vaultMainHelper = result.MainVault
	suite.vaultReplica1Helper = result.Replica1Vault
	suite.vaultReplica2Helper = result.Replica2Vault
	suite.vaultClient, err = vault.NewMultiClusterVaultClient(
		suite.ctx, result.MainConfig, result.ReplicasConfig,
	)
	suite.NoError(err, "Failed to create vault client")
}

func (suite *OrchestratorTestSuite) setupRepositoryClient() {
	var err error
	suite.pgHelper, err = testutil.NewPostgresContainer(suite.T(), suite.ctx)
	suite.NoError(err, "Failed to create Postgres container")
	db, err := db.NewPostgresDatastore(suite.pgHelper.Config, migrations.NewPostgresMigration())
	suite.NoError(err, "Failed to create PostgreSQLSyncedSecretRepository")
	suite.repo = postgres.NewSyncedSecretRepository(db)
}

func (suite *OrchestratorTestSuite) SetupTest() {
	suite.pgHelper.ExecutePsqlCommand(context.Background(), "TRUNCATE TABLE synced_secrets")
	testutil.TruncateSecrets(
		suite.vaultMainHelper, suite.vaultReplica1Helper, suite.vaultReplica2Helper, mounts...,
	)
}

func (suite *OrchestratorTestSuite) SetupSubTest() {
	suite.pgHelper.ExecutePsqlCommand(context.Background(), "TRUNCATE TABLE synced_secrets")
	testutil.TruncateSecrets(
		suite.vaultMainHelper, suite.vaultReplica1Helper, suite.vaultReplica2Helper, mounts...,
	)
}

func (suite *OrchestratorTestSuite) TearDownSuite() {
	testutil.TerminateAllClusters(
		suite.vaultMainHelper, suite.vaultReplica1Helper, suite.vaultReplica2Helper,
	)
	suite.pgHelper.Terminate(suite.ctx)
}

func TestOrchestratorSuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}

	suite.Run(t, new(OrchestratorTestSuite))
}

// Test Cases - Focus on Orchestration Logic
func (suite *OrchestratorTestSuite) TestStartSync_Discovery() {
	suite.Run("discovers secrets matching sync rules", func() {
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, "app1/db", map[string]string{"key": "v1"})
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, "app2/api", map[string]string{"key": "v2"})
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamBMount, "config", map[string]string{"key": "v3"})

		cfg := &config.Config{
			SyncRule:    config.SyncRule{KvMounts: mounts, PathsToReplicate: []string{}},
			Concurrency: 1,
		}
		pathMatcher := pathmatching.NewVaultPathMatcher(suite.vaultClient, &cfg.SyncRule)
		orchestrator := NewSyncOrchestrator(suite.vaultClient, suite.repo, pathMatcher, cfg.Concurrency)

		result, err := orchestrator.StartSync(suite.ctx)

		suite.NoError(err)
		suite.Equal(3, result.TotalSecrets)
		suite.Equal(3, result.SuccessfulSyncs)
		suite.Len(result.JobResults, 3)
	})

	suite.Run("returns empty result when no secrets match", func() {
		cfg := &config.Config{
			SyncRule:    config.SyncRule{KvMounts: mounts, PathsToReplicate: []string{}},
			Concurrency: 1,
		}
		pathMatcher := pathmatching.NewVaultPathMatcher(suite.vaultClient, &cfg.SyncRule)
		orchestrator := NewSyncOrchestrator(suite.vaultClient, suite.repo, pathMatcher, cfg.Concurrency)

		result, err := orchestrator.StartSync(suite.ctx)

		suite.NoError(err)
		suite.Equal(0, result.TotalSecrets)
		suite.Len(result.JobResults, 0)
	})

	suite.Run("processes all secrets regardless of concurrency limit", func() {
		for i := 0; i < 10; i++ {
			suite.vaultMainHelper.WriteSecret(
				suite.ctx, teamAMount, "secret"+string(rune('a'+i)), map[string]string{"k": "v"},
			)
		}

		cfg := &config.Config{
			SyncRule:    config.SyncRule{KvMounts: mounts, PathsToReplicate: []string{}},
			Concurrency: 10,
		}
		pathMatcher := pathmatching.NewVaultPathMatcher(suite.vaultClient, &cfg.SyncRule)
		orchestrator := NewSyncOrchestrator(suite.vaultClient, suite.repo, pathMatcher, cfg.Concurrency)

		result, err := orchestrator.StartSync(suite.ctx)

		suite.NoError(err)
		suite.Equal(10, result.TotalSecrets)
		suite.Len(result.JobResults, 10)

		// Verify job results have expected structure
		for _, jobResult := range result.JobResults {
			suite.NotEmpty(jobResult.Mount)
			suite.NotEmpty(jobResult.KeyPath)
			suite.NotNil(jobResult.Status)
		}
	})

	suite.Run("detects no-op when secret unchanged", func() {
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, "existing", map[string]string{"k": "v"})

		cfg := &config.Config{
			SyncRule:    config.SyncRule{KvMounts: mounts, PathsToReplicate: []string{}},
			Concurrency: 1,
		}
		pathMatcher := pathmatching.NewVaultPathMatcher(suite.vaultClient, &cfg.SyncRule)
		orchestrator := NewSyncOrchestrator(suite.vaultClient, suite.repo, pathMatcher, cfg.Concurrency)

		// First sync
		result1, err := orchestrator.StartSync(suite.ctx)
		suite.NoError(err)
		suite.Equal(1, result1.SuccessfulSyncs)

		// Second sync - no changes
		result2, err := orchestrator.StartSync(suite.ctx)

		suite.NoError(err)
		suite.Equal(1, result2.NoOpSecrets)
		suite.Equal(0, result2.SuccessfulSyncs)
	})

	suite.Run("stops and categorizes cancelled jobs as skipped", func() {
		for i := 0; i < 10; i++ {
			suite.vaultMainHelper.WriteSecret(
				suite.ctx, teamAMount, "secret"+string(rune('a'+i)), map[string]string{"k": "v"},
			)
		}

		cfg := &config.Config{
			SyncRule:    config.SyncRule{KvMounts: mounts, PathsToReplicate: []string{}},
			Concurrency: 2,
		}
		pathMatcher := pathmatching.NewVaultPathMatcher(suite.vaultClient, &cfg.SyncRule)
		orchestrator := NewSyncOrchestrator(suite.vaultClient, suite.repo, pathMatcher, cfg.Concurrency)

		ctx, cancel := context.WithCancel(suite.ctx)
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		result, err := orchestrator.StartSync(ctx)

		suite.Error(err)
		suite.Contains(err.Error(), "sync interrupted")
		suite.NotNil(result)
		suite.Greater(result.SkippedSecrets, 0)

		// Verify categorization still correct
		total := result.SuccessfulSyncs + result.FailedSyncs + result.NoOpSecrets + result.SkippedSecrets
		suite.Equal(result.TotalSecrets, total)
	})

	suite.Run("fails immediately if context already cancelled", func() {
		cfg := &config.Config{
			SyncRule:    config.SyncRule{KvMounts: mounts, PathsToReplicate: []string{}},
			Concurrency: 2,
		}
		pathMatcher := pathmatching.NewVaultPathMatcher(suite.vaultClient, &cfg.SyncRule)
		orchestrator := NewSyncOrchestrator(suite.vaultClient, suite.repo, pathMatcher, cfg.Concurrency)

		ctx, cancel := context.WithCancel(suite.ctx)
		cancel()

		result, err := orchestrator.StartSync(ctx)

		suite.Error(err)
		suite.True(errors.Is(err, context.Canceled))
		suite.Nil(result)
	})

	suite.Run("continues processing when individual jobs fail", func() {
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, "secret1", map[string]string{"k": "v1"})
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, "secret2", map[string]string{"k": "v2"})

		cfg := &config.Config{
			SyncRule:    config.SyncRule{KvMounts: mounts, PathsToReplicate: []string{}},
			Concurrency: 1,
		}
		pathMatcher := pathmatching.NewVaultPathMatcher(suite.vaultClient, &cfg.SyncRule)
		orchestrator := NewSyncOrchestrator(suite.vaultClient, suite.repo, pathMatcher, cfg.Concurrency)

		// Stop replica to cause failures
		timeout := 200 * time.Millisecond
		suite.vaultReplica1Helper.Stop(suite.ctx, &timeout)

		result, err := orchestrator.StartSync(suite.ctx)

		suite.NoError(err, "Orchestrator completes even with failures")
		suite.Equal(2, result.TotalSecrets, "Processes all secrets")
		suite.Len(result.JobResults, 2)
		suite.Greater(result.FailedSyncs, 0, "Some jobs should fail")
	})
}
