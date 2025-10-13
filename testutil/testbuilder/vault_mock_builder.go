package testbuilder

import (
	"context"
	"errors"
	"vault-sync/internal/models"
	"vault-sync/internal/vault"

	"github.com/stretchr/testify/mock"
)

const (
	VaultSecretExists             = "SecretExists"
	VaultReplicaSecretExists      = "ReplicaSecretExists"
	VaultGetSecretMetadata        = "GetSecretMetadata"
	VaultSyncSecretToReplicas     = "SyncSecretToReplicas"
	VaultDeleteSecretFromReplicas = "DeleteSecretFromReplicas"
	VaultGetKeysUnderMount        = "GetKeysUnderMount"
)

type VaultMockBuilder struct {
	ctx      context.Context
	mount    string
	keyPath  string
	clusters []string

	mockVault           *mockVaultClient
	sourceSecretExists  *bool
	replicaSecretExists map[string]*bool
	sourceSecretVersion int64
	vaultSyncResults    []*models.SyncedSecret
	vaultDeleteResults  []*models.SyncSecretDeletionResult
	vaultGetKeysResults map[string][]string
	vaultGetKeysErrors  map[string]error
	vaultErrors         map[string]error
}

func NewVaultMockBuilder(mount, keyPath string, clusters ...string) *VaultMockBuilder {
	return &VaultMockBuilder{
		ctx:      context.Background(),
		mount:    mount,
		keyPath:  keyPath,
		clusters: clusters,

		mockVault:           new(mockVaultClient),
		sourceSecretVersion: 1,
		sourceSecretExists:  nil,
		replicaSecretExists: make(map[string]*bool),

		vaultSyncResults:    make([]*models.SyncedSecret, 0),
		vaultDeleteResults:  make([]*models.SyncSecretDeletionResult, 0),
		vaultGetKeysResults: make(map[string][]string),
		vaultGetKeysErrors:  make(map[string]error),
		vaultErrors:         make(map[string]error),
	}
}

func (b *VaultMockBuilder) WithKeyPath(keyPath string) *VaultMockBuilder {
	b.keyPath = keyPath
	return b
}

func (b *VaultMockBuilder) WithMount(mount string) *VaultMockBuilder {
	b.mount = mount
	return b
}

func (b *VaultMockBuilder) WithClusters(clusters ...string) *VaultMockBuilder {
	b.clusters = clusters
	for _, cluster := range clusters {
		b.replicaSecretExists[cluster] = nil
	}

	return b
}

func (b *VaultMockBuilder) WithGetKeysUnderMount(mount string, keys []string) *VaultMockBuilder {
	b.vaultGetKeysResults[mount] = keys
	return b
}

func (b *VaultMockBuilder) WithGetKeysUnderMountError(mount string, err error) *VaultMockBuilder {
	b.vaultGetKeysErrors[mount] = err
	return b
}

func (b *VaultMockBuilder) WithVaultSecretExists(exists bool) *VaultMockBuilder {
	b.sourceSecretExists = &exists
	return b
}

func (b *VaultMockBuilder) WithVaultSecretExistsError(err error) *VaultMockBuilder {
	b.vaultErrors[VaultSecretExists] = err
	return b
}

func (b *VaultMockBuilder) WithVaultSecretExistsInReplicas(exists bool, cluster ...string) *VaultMockBuilder {
	for _, c := range cluster {
		b.replicaSecretExists[c] = &exists
	}
	return b
}

func (b *VaultMockBuilder) WithVaultSecretExistsInReplicasError(err error) *VaultMockBuilder {
	b.vaultErrors[VaultReplicaSecretExists] = err
	return b
}

func (b *VaultMockBuilder) WithGetSecretMetadata(version int64) *VaultMockBuilder {
	if b.sourceSecretExists != nil && *b.sourceSecretExists {
		b.sourceSecretVersion = version
	} else if b.sourceSecretExists == nil {
		panic("Invoke WithVaultSecretExists before WithGetSecretMetadata")
	}
	return b
}

func (b *VaultMockBuilder) WithGetSecretMetadataError(err error) *VaultMockBuilder {
	b.vaultErrors[VaultGetSecretMetadata] = err
	return b
}

func (b *VaultMockBuilder) WithSyncSecretToReplicas(status models.SyncStatus, version int64, clusters ...string) *VaultMockBuilder {

	for _, cluster := range clusters {
		result := &models.SyncedSecret{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			DestinationCluster: cluster,
			Status:             status,
			SourceVersion:      version,
		}
		b.vaultSyncResults = append(b.vaultSyncResults, result)
	}

	return b
}

func (b *VaultMockBuilder) WithSyncSecretToReplicasError(err error) *VaultMockBuilder {
	b.vaultErrors[VaultSyncSecretToReplicas] = err
	return b
}

func (b *VaultMockBuilder) WithDeleteSecretFromReplicas(status models.SyncStatus, clusters ...string) *VaultMockBuilder {
	if b.sourceSecretExists != nil && !*b.sourceSecretExists {
		for _, cluster := range clusters {
			result := &models.SyncSecretDeletionResult{
				SecretBackend:      b.mount,
				SecretPath:         b.keyPath,
				DestinationCluster: cluster,
				Status:             status,
			}
			b.vaultDeleteResults = append(b.vaultDeleteResults, result)
		}
	} else if b.sourceSecretExists == nil {
		panic("Invoke WithVaultSecretExists before WithDeleteSecretFromReplicas")
	}
	return b
}

func (b *VaultMockBuilder) WithDeleteSecretFromReplicasError(err error) *VaultMockBuilder {
	b.vaultErrors[VaultDeleteSecretFromReplicas] = err
	return b
}

func (b *VaultMockBuilder) SwitchToBuildableStage() *VaultMockBuilder {
	return b
}

func (b *VaultMockBuilder) Build() *mockVaultClient {
	b.mockVault = new(mockVaultClient)

	b.mockVault.On("GetReplicaNames").Return(b.clusters)

	// Setup vault SecretExists mock
	if vaultError, hasError := b.vaultErrors[VaultSecretExists]; hasError {
		b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(false, vaultError)
	} else if b.sourceSecretExists != nil {
		b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(*b.sourceSecretExists, nil)
	} else {
		b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(false, nil)
	}

	// Setup vault SecretExists mock
	if vaultError, hasError := b.vaultErrors[VaultReplicaSecretExists]; hasError {
		b.mockVault.On("SecretExistsInReplica", mock.Anything, mock.Anything, b.mount, b.keyPath).Return(false, vaultError)
	} else {
		for cluster, exists := range b.replicaSecretExists {
			if exists != nil {
				b.mockVault.On("SecretExistsInReplica", mock.Anything, cluster, b.mount, b.keyPath).Return(*exists, nil)
			} else {
				b.mockVault.On("SecretExistsInReplica", mock.Anything, cluster, b.mount, b.keyPath).Return(false, nil)
			}
		}
	}

	// Setup vault GetKeysUnderMount mock
	for mount, keys := range b.vaultGetKeysResults {
		if vaultError, hasError := b.vaultErrors[VaultGetKeysUnderMount]; hasError {
			b.mockVault.On("GetKeysUnderMount", mock.Anything, mount, mock.Anything).Return(nil, vaultError)
		} else {
			b.mockVault.On("GetKeysUnderMount", mock.Anything, mount, mock.Anything).Return(keys, nil)
		}
	}

	// Setup vault GetSecretMetadata mock
	if vaultError, hasError := b.vaultErrors[VaultGetSecretMetadata]; hasError {
		b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
	} else {
		if b.sourceSecretExists != nil {
			if !*b.sourceSecretExists {
				b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(nil, errors.New("secret not found"))
			} else {
				metadata := &vault.VaultSecretMetadataResponse{CurrentVersion: b.sourceSecretVersion}
				b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(metadata, nil)
			}
		}
	}

	// setup vault SyncSecretToReplicas mock if secret exists
	if len(b.vaultSyncResults) > 0 || b.vaultErrors[VaultSyncSecretToReplicas] != nil {
		if vaultError, hasError := b.vaultErrors[VaultSyncSecretToReplicas]; hasError {
			b.mockVault.On("SyncSecretToReplicas", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
		} else {
			b.mockVault.On("SyncSecretToReplicas", mock.Anything, b.mount, b.keyPath).Return(b.vaultSyncResults, nil)
		}

	}

	// Setup vault DeleteSecretFromReplicas mock
	if len(b.vaultDeleteResults) > 0 || b.vaultErrors[VaultDeleteSecretFromReplicas] != nil {
		if vaultError, hasError := b.vaultErrors[VaultDeleteSecretFromReplicas]; hasError {
			b.mockVault.On("DeleteSecretFromReplicas", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
		} else {
			b.mockVault.On("DeleteSecretFromReplicas", mock.Anything, b.mount, b.keyPath).Return(b.vaultDeleteResults, nil)
		}
	}

	return b.mockVault
}
