package vault

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

// VaultSecretResponse represents the response structure for a Vault secret read operation.
// It includes the secret data and metadata.
//
// The `Data` field contains the actual secret data, while the `Metadata` field contains
// metadata about the secret, such as creation time, deletion time, and versioning information.
//
// The `Metadata` field is embedded within the `VaultSecretResponse` struct, allowing for easy access to metadata fields.
// The `CreatedTime` field is a standard time.Time type, while the `DeletionTime` field is a custom `NullableTime` type
// that can represent a time.Time value that may be null.
type VaultSecretResponse struct {
	Data     map[string]interface{}     `json:"data"`
	Metadata VaultSecretEmbededMetadata `json:"metadata"`
}

type VaultSecretEmbededMetadata struct {
	CreatedTime  time.Time    `json:"created_time"`
	DeletionTime NullableTime `json:"deletion_time,omitempty"`
	Destroyed    bool         `json:"destroyed"`
	Version      int64        `json:"version"`
}

// VaultSecretMetadataResponse represents the response structure for a Vault secret metadata read operation.
// It includes metadata about the secret, such as current version, max versions, oldest version,
// creation time, update time, and a map of versions.
type VaultSecretMetadataResponse struct {
	CurrentVersion int64                                 `json:"current_version"`
	MaxVersions    int64                                 `json:"max_versions"`
	OldestVersion  int64                                 `json:"oldest_version"`
	CreatedTime    time.Time                             `json:"created_time"`
	UpdatedTime    time.Time                             `json:"updated_time"`
	Versions       map[string]VaultSecretEmbededMetadata `json:"versions"`
}

// parsing functions to convert Vault responses into our custom types
func parseVaultSecretResponse(data *vault.Response[schema.KvV2ReadResponse]) (*VaultSecretResponse, error) {
	jsonData, err := json.Marshal(data.Data)
	if err != nil {
		return nil, err
	}

	var vaultResponse VaultSecretResponse
	if err := json.Unmarshal(jsonData, &vaultResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to VaultSecretResponse: %w", err)
	}

	return &vaultResponse, nil
}

func parseVaultSecretMetadataResponse(data *vault.Response[schema.KvV2ReadMetadataResponse]) (*VaultSecretMetadataResponse, error) {
	jsonData, err := json.Marshal(data.Data)
	if err != nil {
		return nil, err
	}

	var vaultResponse VaultSecretMetadataResponse
	if err := json.Unmarshal(jsonData, &vaultResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to VaultSecretMetadataResponse: %w", err)
	}

	return &vaultResponse, nil
}

// NullableTime is a custom type that can be used to represent a time.Time value that may be null.
type NullableTime struct {
	*time.Time
}

func (nt *NullableTime) IsNull() bool {
	return nt.Time == nil
}

func (nt *NullableTime) String() string {
	if nt.Time == nil {
		return "null"
	}
	return nt.Time.Format(time.RFC3339)
}

func (nt *NullableTime) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" || string(data) == `""` {
		nt.Time = nil
		return nil
	}

	var timeStr string
	if err := json.Unmarshal(data, &timeStr); err != nil {
		return fmt.Errorf("failed to unmarshal time string: %w", err)
	}

	if timeStr == "" {
		nt.Time = nil
		return nil
	}

	parsedTime, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return fmt.Errorf("failed to parse time %q: %w", timeStr, err)
	}

	nt.Time = &parsedTime
	return nil
}
