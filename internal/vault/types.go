package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	// Error messages that indicate "not found"
	ErrorNotFound404 = "404"
	ErrorNoSuchPath  = "no such path"

	// Log messages
	LogSecretNotFound  = "Secret does not exist in replica cluster"
	LogSyncStarted     = "Starting secret synchronization from main cluster to replicas"
	LogDeletionStarted = "Starting secret deletion from replica clusters"
)

// ************
//
// VaultSecretResponse represents the response structure for a Vault secret read operation.
// It includes the secret data and metadata.
//
// The `Data` field contains the actual secret data, while the `Metadata` field contains
// metadata about the secret, such as creation time, deletion time, and versioning information.
//
// The `Metadata` field is embedded within the `VaultSecretResponse` struct, allowing for easy access to metadata fields.
// The `CreatedTime` field is a standard time.Time type, while the `DeletionTime` field is a custom `NullableTime` type
// that can represent a time.Time value that may be null.
//
// ***********
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
type VaultSecretMetadataResponse struct {
	CurrentVersion int64                                 `json:"current_version"`
	MaxVersions    int64                                 `json:"max_versions"`
	OldestVersion  int64                                 `json:"oldest_version"`
	CreatedTime    time.Time                             `json:"created_time"`
	UpdatedTime    time.Time                             `json:"updated_time"`
	Versions       map[string]VaultSecretEmbededMetadata `json:"versions"`
}

// ***********
//
// NullableTime is a custom type that can be used to represent a time.Time value that may be null.
//
// ***********
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

// ************
//
// syncResultAggregator is a generic type that aggregates results from multiple goroutines operation.
//
// ************
type syncResultAggregator[T any] struct {
	results     []T
	resultsChan chan T
	count       int
}

func newSyncResultAggregator[T any](count int) *syncResultAggregator[T] {
	return &syncResultAggregator[T]{
		results:     make([]T, 0, count),
		resultsChan: make(chan T, count),
		count:       count,
	}
}

func (rc *syncResultAggregator[T]) aggregate(ctx context.Context) ([]T, error) {
	for i := 0; i < rc.count; i++ {
		select {
		case result := <-rc.resultsChan:
			rc.results = append(rc.results, result)
		case <-ctx.Done():
			return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
		}
	}
	return rc.results, nil
}

// ************
//
// syncOperationFunc is a function type that defines the signature for synchronization operations.
//
// ************
type syncOperationFunc[T replicaSyncOperationResult] func(ctx context.Context, mount, keyPath, clusterName string, result T) error
