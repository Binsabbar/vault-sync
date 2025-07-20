package vault

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient("http://localhost:8200", "test-token", "secret")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	
	if client == nil {
		t.Fatal("Expected client, got nil")
	}
	
	if client.prefix != "secret" {
		t.Errorf("Expected prefix 'secret', got '%s'", client.prefix)
	}
}

func TestNewClient_InvalidAddress(t *testing.T) {
	// The Vault API client doesn't validate URL format at creation time
	// It will validate at connection time, so this test should pass
	client, err := NewClient("invalid-url", "test-token", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("Expected client, got nil")
	}
}

func TestBuildPath(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		path     string
		expected string
	}{
		{
			name:     "with prefix",
			prefix:   "secret",
			path:     "myapp/config",
			expected: "secret/myapp/config",
		},
		{
			name:     "without prefix",
			prefix:   "",
			path:     "myapp/config",
			expected: "myapp/config",
		},
		{
			name:     "empty path with prefix",
			prefix:   "secret",
			path:     "",
			expected: "secret/",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{prefix: tt.prefix}
			result := client.buildPath(tt.path)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestSecret(t *testing.T) {
	secret := &Secret{
		Path: "myapp/config",
		Data: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}
	
	if secret.Path != "myapp/config" {
		t.Errorf("Expected path 'myapp/config', got '%s'", secret.Path)
	}
	
	if len(secret.Data) != 2 {
		t.Errorf("Expected 2 data entries, got %d", len(secret.Data))
	}
	
	if secret.Data["key1"] != "value1" {
		t.Errorf("Expected 'value1', got '%v'", secret.Data["key1"])
	}
}