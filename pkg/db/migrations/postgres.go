package migrations

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"vault-sync/pkg/log"
)

type PostgresMigration struct {
	fs fs.FS
}

//go:embed postgres/*.sql
var PostgresFS embed.FS

func NewPostgresMigration() *PostgresMigration {
	subFS, err := fs.Sub(PostgresFS, "postgres")
	if err != nil {
		log.Logger.Error().Err(err).Msg("Failed to create sub filesystem for Postgres migrations")
		return nil
	}
	return &PostgresMigration{
		fs: subFS,
	}
}

func (p *PostgresMigration) GetSourceType() string {
	return "iofs"
}

func (p *PostgresMigration) GetSourceDriver() (source.Driver, error) {
	d, err := iofs.New(p.fs, ".")
	if err != nil {
		log.Logger.Error().Err(err).Msg("Failed to create migration source from embedded files")
		return nil, fmt.Errorf("failed to create migration source: %w", err)
	}
	return d, nil
}
