package vault

import (
	"fmt"

	"github.com/hashicorp/vault/api"
)

// Client wraps the Vault API client with additional functionality
type Client struct {
	client *api.Client
	prefix string
}

// NewClient creates a new Vault client
func NewClient(address, token, prefix string) (*Client, error) {
	config := api.DefaultConfig()
	config.Address = address

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	client.SetToken(token)

	return &Client{
		client: client,
		prefix: prefix,
	}, nil
}

// Secret represents a secret from Vault
type Secret struct {
	Path string
	Data map[string]interface{}
}

// ListSecrets lists all secrets under the specified path
func (c *Client) ListSecrets(path string) ([]string, error) {
	fullPath := c.buildPath(path)
	
	secret, err := c.client.Logical().List(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets at %s: %w", fullPath, err)
	}

	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return []string{}, nil
	}

	var secretPaths []string
	for _, key := range keys {
		if keyStr, ok := key.(string); ok {
			secretPaths = append(secretPaths, keyStr)
		}
	}

	return secretPaths, nil
}

// ReadSecret reads a secret from Vault
func (c *Client) ReadSecret(path string) (*Secret, error) {
	fullPath := c.buildPath(path)
	
	secret, err := c.client.Logical().Read(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret at %s: %w", fullPath, err)
	}

	if secret == nil {
		return nil, fmt.Errorf("secret not found at %s", fullPath)
	}

	return &Secret{
		Path: path,
		Data: secret.Data,
	}, nil
}

// WriteSecret writes a secret to Vault
func (c *Client) WriteSecret(secret *Secret) error {
	fullPath := c.buildPath(secret.Path)
	
	_, err := c.client.Logical().Write(fullPath, secret.Data)
	if err != nil {
		return fmt.Errorf("failed to write secret to %s: %w", fullPath, err)
	}

	return nil
}

func (c *Client) buildPath(path string) string {
	if c.prefix == "" {
		return path
	}
	return c.prefix + "/" + path
}