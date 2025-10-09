package testbuilder

import (
	"context"
	"vault-sync/internal/models"
	"vault-sync/internal/vault"

	"github.com/stretchr/testify/mock"
)

// ********
//
// mockVaultClient is a mock implementation of the VaultSyncer interface
//
// ********
type mockVaultClient struct {
	mock.Mock
}

func (m *mockVaultClient) GetReplicaNames() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *mockVaultClient) GetSecretMounts(ctx context.Context, secretPaths []string) ([]string, error) {
	args := m.Called(ctx, secretPaths)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockVaultClient) GetSecretMetadata(ctx context.Context, mount, keyPath string) (*vault.VaultSecretMetadataResponse, error) {
	args := m.Called(ctx, mount, keyPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*vault.VaultSecretMetadataResponse), args.Error(1)
}

func (m *mockVaultClient) GetKeysUnderMount(ctx context.Context, mount string, shouldIncludeKeyPath func(path string, isFinalPath bool) bool) ([]string, error) {
	args := m.Called(ctx, mount, shouldIncludeKeyPath)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockVaultClient) SecretExists(ctx context.Context, mount, keyPath string) (bool, error) {
	args := m.Called(ctx, mount, keyPath)
	return args.Bool(0), args.Error(1)
}

func (m *mockVaultClient) SyncSecretToReplicas(ctx context.Context, mount, keyPath string) ([]*models.SyncedSecret, error) {
	args := m.Called(ctx, mount, keyPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.SyncedSecret), args.Error(1)
}

func (m *mockVaultClient) DeleteSecretFromReplicas(ctx context.Context, mount, keyPath string) ([]*models.SyncSecretDeletionResult, error) {
	args := m.Called(ctx, mount, keyPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.SyncSecretDeletionResult), args.Error(1)
}

// ********
//
// mockRepository is a mock implementation of the SyncedSecretRepository interface
//
// ********
type mockRepository struct {
	mock.Mock
}

func (m *mockRepository) GetSyncedSecret(mount, keyPath, clusterName string) (*models.SyncedSecret, error) {
	args := m.Called(mount, keyPath, clusterName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.SyncedSecret), args.Error(1)
}

func (m *mockRepository) UpdateSyncedSecretStatus(secret *models.SyncedSecret) error {
	args := m.Called(secret)
	return args.Error(0)
}
func (m *mockRepository) GetSyncedSecrets() ([]*models.SyncedSecret, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.SyncedSecret), args.Error(1)
}
func (m *mockRepository) DeleteSyncedSecret(backend, path, destinationCluster string) error {
	args := m.Called(backend, path, destinationCluster)
	return args.Error(0)
}

func (m *mockRepository) Close() error {
	args := m.Called()
	return args.Error(0)
}
