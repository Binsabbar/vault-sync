package orchestrator

import (
	"context"
	"errors"
	"fmt"
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

func (suite *OrchestratorTestSuite) TestStartSync_DeletesSecretsNotInMaster() {
	suite.Run("deletes secrets from replicas when deleted from master", func() {
		// Setup: Create secrets in master and sync them
		suite.writeSecretsToMaster(teamAMount, "app1/db", "app2/api")
		cfg := &config.Config{SyncRule: config.SyncRule{KvMounts: mounts}, Concurrency: 1}
		orchestrator := suite.createOrchestrator(cfg)

		// First sync - sync both secrets to replicas
		result1, err := orchestrator.StartSync(suite.ctx)
		suite.NoError(err)
		suite.Equal(2, result1.SuccessfulSyncs)

		// Verify initial state
		suite.assertSecretExistsInReplicas(teamAMount, "app1/db")
		suite.assertDBRecordCount(4, "Should have 2 secrets × 2 replicas")

		// Delete one secret from master
		suite.deleteSecretFromMaster(teamAMount, "app1/db")

		// Second sync - should delete from replicas
		result2, err := orchestrator.StartSync(suite.ctx)
		suite.NoError(err)
		suite.Equal(2, result2.TotalSecrets)
		suite.Equal(1, result2.SuccessfulSyncs, "Should delete one secret")
		suite.Equal(1, result2.NoOpSecrets, "Other secret unchanged")

		// Verify deletion
		suite.assertSecretDeletedFromReplicas(teamAMount, "app1/db")
		suite.assertDBRecordCount(2, "Should have 1 secret × 2 replicas after deletion")
		suite.assertOnlySecretRemains("app2/api")
	})

	suite.Run("handles multiple deletions at once", func() {
		suite.writeSecretsToMaster(teamAMount, "secret1", "secret2", "secret3")
		cfg := &config.Config{SyncRule: config.SyncRule{KvMounts: mounts}, Concurrency: 1}
		orchestrator := suite.createOrchestrator(cfg)

		// First sync
		result1, err := orchestrator.StartSync(suite.ctx)
		suite.NoError(err)
		suite.Equal(3, result1.SuccessfulSyncs)

		// Delete 2 secrets from master
		suite.deleteSecretFromMaster(teamAMount, "secret1", "secret2")

		// Second sync
		result2, err := orchestrator.StartSync(suite.ctx)
		suite.NoError(err)
		suite.Equal(3, result2.TotalSecrets)
		suite.Equal(2, result2.SuccessfulSyncs, "Should delete 2 secrets")
		suite.Equal(1, result2.NoOpSecrets, "1 secret unchanged")

		// Verify only secret3 remains
		suite.assertSecretExistsInReplicas(teamAMount, "secret3")
		suite.assertSecretDeletedFromReplicas(teamAMount, "secret1", "secret2")
	})

	suite.Run("does not delete secrets that were never synced", func() {
		// Create orphan secret only in replica
		suite.vaultReplica1Helper.WriteSecret(suite.ctx, teamAMount, "orphan-secret", map[string]string{"k": "v"})
		suite.vaultReplica2Helper.WriteSecret(suite.ctx, teamAMount, "orphan-secret", map[string]string{"k": "v"})

		// Create and sync master secret
		suite.writeSecretsToMaster(teamAMount, "master-secret")
		cfg := &config.Config{SyncRule: config.SyncRule{KvMounts: mounts}, Concurrency: 1}
		orchestrator := suite.createOrchestrator(cfg)

		result, err := orchestrator.StartSync(suite.ctx)
		suite.NoError(err)
		suite.Equal(1, result.TotalSecrets, "Should only process master-secret")
		suite.Equal(1, result.SuccessfulSyncs)

		// Verify orphan remains untouched
		suite.assertSecretExistsInReplicas(teamAMount, "orphan-secret")
	})
}

// Helper methods
func (suite *OrchestratorTestSuite) createOrchestrator(cfg *config.Config) *SyncOrchestrator {
	pathMatcher := pathmatching.NewVaultPathMatcher(suite.vaultClient, &cfg.SyncRule)
	return NewSyncOrchestrator(suite.vaultClient, suite.repo, pathMatcher, cfg.Concurrency)
}

func (suite *OrchestratorTestSuite) writeSecretsToMaster(mount string, paths ...string) {
	for i, path := range paths {
		suite.vaultMainHelper.WriteSecret(
			suite.ctx, mount, path,
			map[string]string{"key": fmt.Sprintf("v%d", i+1)},
		)
	}
}

func (suite *OrchestratorTestSuite) deleteSecretFromMaster(mount string, paths ...string) {
	for _, path := range paths {
		_, err := suite.vaultMainHelper.DeleteSecret(suite.ctx, fmt.Sprintf("%s/%s", mount, path))
		suite.NoError(err, "Failed to delete secret %s from master", path)
	}
}

func (suite *OrchestratorTestSuite) assertSecretExistsInReplicas(mount string, paths ...string) {
	for _, path := range paths {
		_, _, err := suite.vaultReplica1Helper.ReadSecretData(suite.ctx, mount, path)
		suite.NoError(err, "Secret %s should exist in replica1", path)
		_, _, err = suite.vaultReplica2Helper.ReadSecretData(suite.ctx, mount, path)
		suite.NoError(err, "Secret %s should exist in replica2", path)
	}
}

func (suite *OrchestratorTestSuite) assertSecretDeletedFromReplicas(mount string, paths ...string) {
	for _, path := range paths {
		_, _, err := suite.vaultReplica1Helper.ReadSecretData(suite.ctx, mount, path)
		suite.Error(err, "Secret %s should be deleted from replica1", path)
		_, _, err = suite.vaultReplica2Helper.ReadSecretData(suite.ctx, mount, path)
		suite.Error(err, "Secret %s should be deleted from replica2", path)
	}
}

func (suite *OrchestratorTestSuite) assertDBRecordCount(expected int, msgAndArgs ...interface{}) {
	syncedSecrets, err := suite.repo.GetSyncedSecrets()
	suite.NoError(err)
	suite.Len(syncedSecrets, expected, msgAndArgs...)
}

func (suite *OrchestratorTestSuite) assertOnlySecretRemains(expectedPath string) {
	syncedSecrets, err := suite.repo.GetSyncedSecrets()
	suite.NoError(err)
	for _, record := range syncedSecrets {
		suite.Equal(expectedPath, record.SecretPath, "Only %s should remain in DB", expectedPath)
	}
}
