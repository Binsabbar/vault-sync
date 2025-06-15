package vault

import (
	"time"
)

type SecretMetadata struct {
	Version     int
	CreatedTime time.Time
	UpdatedTime time.Time
}

type Secret struct {
	Data     map[string]interface{}
	Metadata SecretMetadata
}
