package vault

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

type VaultSecretResponse struct {
	Data     VaultSecretData
	Metadata VaultSecretEmbededMetadata
}

type VaultSecretData struct {
	Data map[string]interface{} `json:"data"`
}

type VaultSecretEmbededMetadata struct {
	CreatedTime  time.Time    `json:"created_time"`
	DeletionTime NullableTime `json:"deletion_time,omitempty"`
	Destroyed    bool         `json:"destroyed"`
	Version      int64        `json:"version"`
}

type VaultSecretMetadataResponse struct {
	CurrentVersion int64                         `json:"current_version"`
	MaxVersions    int64                         `json:"max_versions"`
	OldestVersion  int64                         `json:"oldest_version"`
	CreatedTime    time.Time                     `json:"created_time"`
	UpdatedTime    time.Time                     `json:"updated_time"`
	Versions       map[string]VaultSecretVersion `json:"versions"`
}

type VaultSecretVersion struct {
	CreatedTime  time.Time    `json:"created_time"`
	DeletionTime NullableTime `json:"deletion_time,omitempty"`
	Destroyed    bool         `json:"destroyed"`
	Version      string       `json:"version"`
}

type NullableTime struct {
	*time.Time
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
		fmt.Println("jsonData:", string(jsonData))
		return nil, fmt.Errorf("failed to unmarshal JSON to VaultSecretMetadataResponse: %w", err)
	}

	return &vaultResponse, nil
}
