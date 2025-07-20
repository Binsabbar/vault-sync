package sync

import (
	"testing"

	"github.com/Binsabbar/vault-sync/internal/config"
	"github.com/Binsabbar/vault-sync/internal/vault"
)

func TestNew_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		Source: config.VaultConfig{
			Address: "http://localhost:8200",
			Token:   "test-token",
		},
		Target: config.VaultConfig{
			Address: "http://localhost:8201",
			Token:   "test-token",
		},
	}

	syncer, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if syncer == nil {
		t.Fatal("Expected syncer, got nil")
	}

	if syncer.sourceClient == nil {
		t.Fatal("Expected source client, got nil")
	}

	if syncer.targetClient == nil {
		t.Fatal("Expected target client, got nil")
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	cfg := &config.Config{
		Source: config.VaultConfig{
			Address: "",
			Token:   "test-token",
		},
		Target: config.VaultConfig{
			Address: "http://localhost:8201",
			Token:   "test-token",
		},
	}

	_, err := New(cfg)
	if err == nil {
		t.Fatal("Expected error for invalid config, got nil")
	}
}

func TestNewWithClients(t *testing.T) {
	mockSource := &MockVaultClient{}
	mockTarget := &MockVaultClient{}

	syncer := NewWithClients(mockSource, mockTarget)
	if syncer == nil {
		t.Fatal("Expected syncer, got nil")
	}

	if syncer.sourceClient != mockSource {
		t.Fatal("Expected source client to match")
	}

	if syncer.targetClient != mockTarget {
		t.Fatal("Expected target client to match")
	}
}

// MockVaultClient for testing
type MockVaultClient struct {
	secrets       []string
	secretData    map[string]*vault.Secret
	syncedSecrets []string
}

func (m *MockVaultClient) ListSecrets(path string) ([]string, error) {
	return m.secrets, nil
}

func (m *MockVaultClient) ReadSecret(path string) (*vault.Secret, error) {
	if secret, exists := m.secretData[path]; exists {
		return secret, nil
	}
	return &vault.Secret{
		Path: path,
		Data: map[string]interface{}{"key": "value"},
	}, nil
}

func (m *MockVaultClient) WriteSecret(secret *vault.Secret) error {
	m.syncedSecrets = append(m.syncedSecrets, secret.Path)
	return nil
}

func TestSync_DryRun(t *testing.T) {
	mockSource := &MockVaultClient{
		secrets: []string{"secret1", "secret2"},
	}
	mockTarget := &MockVaultClient{}

	syncer := NewWithClients(mockSource, mockTarget)

	err := syncer.Sync(true)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if len(mockTarget.syncedSecrets) != 0 {
		t.Errorf("Expected no synced secrets in dry run, got %d", len(mockTarget.syncedSecrets))
	}
}

func TestSync_ActualRun(t *testing.T) {
	mockSource := &MockVaultClient{
		secrets: []string{"secret1", "secret2"},
	}
	mockTarget := &MockVaultClient{}

	syncer := NewWithClients(mockSource, mockTarget)

	err := syncer.Sync(false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if len(mockTarget.syncedSecrets) != 2 {
		t.Errorf("Expected 2 synced secrets, got %d", len(mockTarget.syncedSecrets))
	}

	expectedSecrets := []string{"secret1", "secret2"}
	for i, secret := range mockTarget.syncedSecrets {
		if secret != expectedSecrets[i] {
			t.Errorf("Expected secret '%s', got '%s'", expectedSecrets[i], secret)
		}
	}
}

func TestSyncPath(t *testing.T) {
	mockSource := &MockVaultClient{
		secrets: []string{"subsecret1", "subsecret2"},
	}
	mockTarget := &MockVaultClient{}

	syncer := NewWithClients(mockSource, mockTarget)

	err := syncer.SyncPath("app/config", false)
	if err != nil {
		t.Fatalf("SyncPath failed: %v", err)
	}

	if len(mockTarget.syncedSecrets) != 2 {
		t.Errorf("Expected 2 synced secrets, got %d", len(mockTarget.syncedSecrets))
	}

	expected := []string{"app/config/subsecret1", "app/config/subsecret2"}
	for i, secret := range mockTarget.syncedSecrets {
		if secret != expected[i] {
			t.Errorf("Expected secret '%s', got '%s'", expected[i], secret)
		}
	}
}

func TestSyncSecret(t *testing.T) {
	secretData := map[string]*vault.Secret{
		"test/secret": {
			Path: "test/secret",
			Data: map[string]interface{}{"key": "value"},
		},
	}

	mockSource := &MockVaultClient{
		secretData: secretData,
	}
	mockTarget := &MockVaultClient{}

	syncer := NewWithClients(mockSource, mockTarget)

	err := syncer.SyncSecret("test/secret", false)
	if err != nil {
		t.Fatalf("SyncSecret failed: %v", err)
	}

	if len(mockTarget.syncedSecrets) != 1 {
		t.Errorf("Expected 1 synced secret, got %d", len(mockTarget.syncedSecrets))
	}

	if mockTarget.syncedSecrets[0] != "test/secret" {
		t.Errorf("Expected secret 'test/secret', got '%s'", mockTarget.syncedSecrets[0])
	}
}