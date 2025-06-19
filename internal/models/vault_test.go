package models

import (
	"testing"
	"time"

	"github.com/hashicorp/vault-client-go/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKvV2ReadMetadataResponseToVaultSecretMetadata(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	type TestData struct {
		name        string
		input       schema.KvV2ReadMetadataResponse
		expected    *VaultSecretMetadata
		expectError bool
		errorMsg    string
	}
	tests := []TestData{
		{
			name: "successful parsing with single version",
			input: schema.KvV2ReadMetadataResponse{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions: map[string]interface{}{
					"1": map[string]interface{}{
						"created_time":  "2025-06-19T07:40:04.696796386Z",
						"deletion_time": "2025-06-19T07:40:04.696796386Z",
						"destroyed":     false,
					},
				},
			},
			expected: &VaultSecretMetadata{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions: map[string]VaultSecretVersion{
					"1": {
						CreatedTime:  time.Date(2025, 6, 19, 7, 40, 4, 696796386, time.UTC),
						DeletionTime: func() *time.Time { t := time.Date(2025, 6, 19, 7, 40, 4, 696796386, time.UTC); return &t }(),
						Destroyed:    false,
						Version:      "1",
					},
				},
			},
			expectError: false,
		},
		{
			name: "successful parsing if deletion_time is empty",
			input: schema.KvV2ReadMetadataResponse{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions: map[string]interface{}{
					"1": map[string]interface{}{
						"created_time":  "2025-06-19T07:40:04.696796386Z",
						"deletion_time": "",
						"destroyed":     false,
					},
				},
			},
			expected: &VaultSecretMetadata{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions: map[string]VaultSecretVersion{
					"1": {
						CreatedTime:  time.Date(2025, 6, 19, 7, 40, 4, 696796386, time.UTC),
						DeletionTime: nil,
						Destroyed:    false,
						Version:      "1",
					},
				},
			},
			expectError: false,
		},
		{
			name: "successful parsing with multiple versions",
			input: schema.KvV2ReadMetadataResponse{
				CurrentVersion: 3,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime.Add(2 * time.Hour),
				Versions: map[string]interface{}{
					"1": map[string]interface{}{
						"created_time":  "2025-06-14T07:40:04.696796386Z",
						"deletion_time": "2025-06-15T07:40:04.696796386Z",
						"destroyed":     true,
					},
					"2": map[string]interface{}{
						"created_time":  "2025-06-15T07:40:04.696796386Z",
						"deletion_time": "2025-06-16T07:40:04.696796386Z",
						"destroyed":     false,
					},
					"3": map[string]interface{}{
						"created_time":  "2025-06-18T07:40:04.696796386Z",
						"deletion_time": "2025-06-19T07:40:04.696796386Z",
						"destroyed":     false,
					},
				},
			},
			expected: &VaultSecretMetadata{
				CurrentVersion: 3,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime.Add(2 * time.Hour),
				Versions: map[string]VaultSecretVersion{
					"1": {
						CreatedTime:  time.Date(2025, 6, 14, 7, 40, 4, 696796386, time.UTC),
						DeletionTime: func() *time.Time { t := time.Date(2025, 6, 15, 7, 40, 4, 696796386, time.UTC); return &t }(),
						Destroyed:    true,
						Version:      "1",
					},
					"2": {
						CreatedTime:  time.Date(2025, 6, 15, 7, 40, 4, 696796386, time.UTC),
						DeletionTime: func() *time.Time { t := time.Date(2025, 6, 16, 7, 40, 4, 696796386, time.UTC); return &t }(),
						Destroyed:    false,
						Version:      "2",
					},
					"3": {
						CreatedTime:  time.Date(2025, 6, 18, 7, 40, 4, 696796386, time.UTC),
						DeletionTime: func() *time.Time { t := time.Date(2025, 6, 19, 7, 40, 4, 696796386, time.UTC); return &t }(),
						Destroyed:    false,
						Version:      "3",
					},
				},
			},
			expectError: false,
		},
		{
			name: "empty versions map",
			input: schema.KvV2ReadMetadataResponse{
				CurrentVersion: 0,
				MaxVersions:    10,
				OldestVersion:  0,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions:       map[string]interface{}{},
			},
			expected: &VaultSecretMetadata{
				CurrentVersion: 0,
				MaxVersions:    10,
				OldestVersion:  0,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions:       map[string]VaultSecretVersion{},
			},
			expectError: false,
		},
		{
			name: "invalid version data type",
			input: schema.KvV2ReadMetadataResponse{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions: map[string]interface{}{
					"1": "invalid-string-instead-of-map",
				},
			},
			expected: &VaultSecretMetadata{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions:       map[string]VaultSecretVersion{},
			},
			expectError: false,
		},
		{
			name: "invalid field type in created_time",
			input: schema.KvV2ReadMetadataResponse{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions: map[string]interface{}{
					"1": map[string]interface{}{
						"created_time":  "invalid-time-string",
						"deletion_time": "2025-06-19T07:40:04.696796386Z",
						"destroyed":     false,
					},
				},
			},
			expectError: true,
			errorMsg:    "failed to parse created_time as time.Time",
		},
		{
			name: "invalid field type in deletion_time",
			input: schema.KvV2ReadMetadataResponse{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions: map[string]interface{}{
					"1": map[string]interface{}{
						"created_time":  "2025-06-19T07:40:04.696796386Z",
						"deletion_time": "invalid-time-string",
						"destroyed":     false,
					},
				},
			},
			expectError: true,
			errorMsg:    "failed to parse deletion_time as time.Time",
		},
		{
			name: "invalid field type in version data",
			input: schema.KvV2ReadMetadataResponse{
				CurrentVersion: 1,
				MaxVersions:    10,
				OldestVersion:  1,
				CreatedTime:    baseTime,
				UpdatedTime:    baseTime,
				Versions: map[string]interface{}{
					"1": map[string]interface{}{
						"created_time":  "2025-06-19T07:40:04.696796386Z",
						"deletion_time": "2025-06-19T07:40:04.696796386Z",
						"destroyed":     "not_correct_type",
					},
				},
			},
			expectError: true,
			errorMsg:    "failed to parse destroyed as bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseKvV2ReadMetadataResponseToVaultSecretMetadata(tt.input)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.CurrentVersion, result.CurrentVersion)
				assert.Equal(t, tt.expected.MaxVersions, result.MaxVersions)
				assert.Equal(t, tt.expected.OldestVersion, result.OldestVersion)
				assert.Equal(t, tt.expected.CreatedTime, result.CreatedTime)
				assert.Equal(t, tt.expected.UpdatedTime, result.UpdatedTime)
				assert.Equal(t, len(tt.expected.Versions), len(result.Versions))
				assert.Equal(t, tt.expected.Versions, result.Versions)
				// for k, expectedVersion := range tt.expected.Versions {
				// 	actualVersion, exists := result.Versions[k]
				// 	assert.True(t, exists, "Version %s should exist", k)
				// 	assert.Equal(t, expectedVersion, actualVersion)
				// }
			}
		})
	}
}
