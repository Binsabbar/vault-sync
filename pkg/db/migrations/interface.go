package migrations

import (
	"github.com/golang-migrate/migrate/v4/source"
)

type MigrationSource interface {
	GetSourceType() string
	GetSourceDriver() (source.Driver, error)
}
