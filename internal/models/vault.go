package models

import (
	"fmt"
	"time"

	"github.com/hashicorp/vault-client-go/schema"
)

type ParsingError struct {
	error
	Message string
}

func (e *ParsingError) Error() string {
	return e.Message
}

type VaultSecretMetadata struct {
	CurrentVersion int64                         `json:"current_version"`
	MaxVersions    int64                         `json:"max_versions"`
	OldestVersion  int64                         `json:"oldest_version"`
	CreatedTime    time.Time                     `json:"created_time"`
	UpdatedTime    time.Time                     `json:"updated_time"`
	Versions       map[string]VaultSecretVersion `json:"versions"`
}

type VaultSecretVersion struct {
	CreatedTime  time.Time  `json:"created_time"`
	DeletionTime *time.Time `json:"deletion_time,omitempty"`
	Destroyed    bool       `json:"destroyed"`
	Version      string     `json:"version"`
}

func ParseKvV2ReadMetadataResponseToVaultSecretMetadata(data schema.KvV2ReadMetadataResponse) (*VaultSecretMetadata, error) {
	v := &VaultSecretMetadata{
		CurrentVersion: data.CurrentVersion,
		MaxVersions:    data.MaxVersions,
		OldestVersion:  data.OldestVersion,
		CreatedTime:    data.CreatedTime,
		UpdatedTime:    data.UpdatedTime,
		Versions:       make(map[string]VaultSecretVersion),
	}

	for k, version := range data.Versions {
		if versionData, ok := version.(map[string]interface{}); ok {
			createdTime, err := ParseTimePtr(versionData, "created_time", false)
			if err != nil {
				return nil, err
			}
			deletionTime, err := ParseTimePtr(versionData, "deletion_time", true)
			if err != nil {
				return nil, err
			}
			destroyed, err := ParseBool(versionData, "destroyed")
			if err != nil {
				return nil, err
			}
			version := k
			v.Versions[version] = VaultSecretVersion{
				CreatedTime:  *createdTime,
				DeletionTime: deletionTime,
				Destroyed:    destroyed,
				Version:      version,
			}
		}
	}
	return v, nil
}

func ParseTimePtr(data map[string]interface{}, key string, ignoreEmptyString bool) (*time.Time, error) {
	if v, ok := data[key]; ok {
		switch v := v.(type) {
		case string:
			if ignoreEmptyString && v == "" {
				return nil, nil
			}
			parsedTime, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, &ParsingError{Message: "failed to parse " + key + " as time.Time: " + err.Error()}
			}
			return &parsedTime, nil
		default:
			return nil, &ParsingError{Message: "failed to parse " + key + " as time.Time, unsupported type: " + fmt.Sprintf("%T", v)}
		}
	} else {
		return nil, nil
	}
}

func ParseBool(data map[string]interface{}, key string) (bool, error) {
	v, ok := data[key].(bool)
	if !ok {
		return false, &ParsingError{Message: "failed to parse " + key + " as bool"}
	}
	return v, nil
}
