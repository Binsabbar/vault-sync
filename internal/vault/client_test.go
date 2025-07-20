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

func TestNewClient_EmptyAddress(t *testing.T) {
	_, err := NewClient("", "test-token", "")
	if err == nil {
		t.Fatal("Expected error for empty address, got nil")
	}
}

func TestNewClient_EmptyToken(t *testing.T) {
	_, err := NewClient("http://localhost:8200", "", "")
	if err == nil {
		t.Fatal("Expected error for empty token, got nil")
	}
}

func TestNewClient_ValidConfig(t *testing.T) {
	client, err := NewClient("http://localhost:8200", "test-token", "")
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
			expected: "secret",
		},
		{
			name:     "empty path without prefix",
			prefix:   "",
			path:     "",
			expected: "",
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

func TestSecret_Methods(t *testing.T) {
	secret := &Secret{
		Path: "myapp/config",
		Data: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	}
	
	// Test basic properties
	if secret.Path != "myapp/config" {
		t.Errorf("Expected path 'myapp/config', got '%s'", secret.Path)
	}
	
	if len(secret.Data) != 2 {
		t.Errorf("Expected 2 data entries, got %d", len(secret.Data))
	}
	
	// Test IsEmpty
	if secret.IsEmpty() {
		t.Error("Expected secret to not be empty")
	}
	
	// Test GetValue
	value, exists := secret.GetValue("key1")
	if !exists {
		t.Error("Expected key1 to exist")
	}
	if value != "value1" {
		t.Errorf("Expected 'value1', got '%v'", value)
	}
	
	_, exists = secret.GetValue("nonexistent")
	if exists {
		t.Error("Expected nonexistent key to not exist")
	}
	
	// Test SetValue
	secret.SetValue("key3", "value3")
	if len(secret.Data) != 3 {
		t.Errorf("Expected 3 data entries after SetValue, got %d", len(secret.Data))
	}
	
	value, exists = secret.GetValue("key3")
	if !exists || value != "value3" {
		t.Errorf("Expected key3 to be 'value3', got '%v'", value)
	}
}

func TestSecret_IsEmpty(t *testing.T) {
	emptySecret := &Secret{
		Path: "test",
		Data: map[string]interface{}{},
	}
	
	if !emptySecret.IsEmpty() {
		t.Error("Expected empty secret to be empty")
	}
	
	nilDataSecret := &Secret{
		Path: "test",
		Data: nil,
	}
	
	if !nilDataSecret.IsEmpty() {
		t.Error("Expected nil data secret to be empty")
	}
}

func TestSecret_SetValueOnNilData(t *testing.T) {
	secret := &Secret{
		Path: "test",
		Data: nil,
	}
	
	secret.SetValue("key", "value")
	
	if secret.Data == nil {
		t.Fatal("Expected Data to be initialized")
	}
	
	if len(secret.Data) != 1 {
		t.Errorf("Expected 1 data entry, got %d", len(secret.Data))
	}
	
	value, exists := secret.GetValue("key")
	if !exists || value != "value" {
		t.Errorf("Expected key to be 'value', got '%v'", value)
	}
}