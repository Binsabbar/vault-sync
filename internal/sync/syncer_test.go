package sync

import (
	"testing"

	"github.com/Binsabbar/vault-sync/internal/config"
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

	syncer := New(cfg)
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

// MockSyncer for testing without actual Vault connections
type MockSyncer struct {
	secrets []string
	dryRun  bool
	syncedSecrets []string
}

func (m *MockSyncer) Sync(dryRun bool) error {
	m.dryRun = dryRun
	for _, secret := range m.secrets {
		if !dryRun {
			m.syncedSecrets = append(m.syncedSecrets, secret)
		}
	}
	return nil
}

func TestMockSync_DryRun(t *testing.T) {
	mock := &MockSyncer{
		secrets: []string{"secret1", "secret2"},
	}

	err := mock.Sync(true)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if !mock.dryRun {
		t.Error("Expected dry run to be true")
	}

	if len(mock.syncedSecrets) != 0 {
		t.Errorf("Expected no synced secrets in dry run, got %d", len(mock.syncedSecrets))
	}
}

func TestMockSync_ActualRun(t *testing.T) {
	mock := &MockSyncer{
		secrets: []string{"secret1", "secret2"},
	}

	err := mock.Sync(false)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if mock.dryRun {
		t.Error("Expected dry run to be false")
	}

	if len(mock.syncedSecrets) != 2 {
		t.Errorf("Expected 2 synced secrets, got %d", len(mock.syncedSecrets))
	}

	expectedSecrets := []string{"secret1", "secret2"}
	for i, secret := range mock.syncedSecrets {
		if secret != expectedSecrets[i] {
			t.Errorf("Expected secret '%s', got '%s'", expectedSecrets[i], secret)
		}
	}
}