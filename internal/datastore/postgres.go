package datastore

import (
	"fmt"
	"path/filepath"

	"net/url"
	"vault-sync/internal/config"
	"vault-sync/internal/log"

	"github.com/golang-migrate/migrate/v4"
	psqlmigrator "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type PostgresDatastore struct {
	db *sqlx.DB
}

func NewPostgresDatastore(cfg config.Postgres) (*PostgresDatastore, error) {
	sslMode := cfg.SSLMode
	if cfg.SSLMode == "" {
		sslMode = "disable"
	}
	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.Username, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Address, cfg.Port),
		Path:   cfg.DBName,
	}
	query := dsn.Query()
	query.Set("sslmode", sslMode)
	dsn.RawQuery = query.Encode()

	connectionString := dsn.String()
	redactedConnectionString := redactDSN(connectionString, cfg)
	log.Logger.Info().Str("dsn", redactedConnectionString).Msg("Attempting to connect to PostgreSQL")

	db, err := sqlx.Connect("pgx", connectionString)
	if err != nil {
		log.Logger.Error().Err(err).Str("dsn", redactedConnectionString).Msg("Failed to connect to PostgreSQL")
		return nil, fmt.Errorf("failed to connect to postgres using connectionString '%s': %w", redactedConnectionString, err)
	}

	if err = db.Ping(); err != nil {
		db.Close()
		log.Logger.Error().Err(err).Str("dsn", redactedConnectionString).Msg("Failed to ping PostgreSQL")
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	log.Logger.Info().Str("dsn", redactedConnectionString).Msg("Successfully connected to PostgreSQL")

	if cfg.MaxConnections > 0 {
		db.SetMaxOpenConns(cfg.MaxConnections)
		log.Logger.Info().Int("max_connections", cfg.MaxConnections).Msg("Set max open connections for PostgreSQL")
	}

	return &PostgresDatastore{db: db}, nil
}

func (p *PostgresDatastore) InitSchema(migrationsPath string) error {
	log.Logger.Info().Str("migrations_path", migrationsPath).Msg("Initializing database schema via migrations...")

	absMigrationsPath, err := filepath.Abs(migrationsPath)
	if err != nil {
		log.Logger.Error().Err(err).Str("path", migrationsPath).Msg("Failed to get absolute path for migrations")
		return fmt.Errorf("failed to get absolute path for migrations at %s: %w", migrationsPath, err)
	}

	sourceURL := fmt.Sprintf("file://%s", absMigrationsPath)
	driver, err := psqlmigrator.WithInstance(p.db.DB, &psqlmigrator.Config{})
	if err != nil {
		log.Logger.Error().Err(err).Msg("Could not create postgres driver for migrate")
		return fmt.Errorf("could not create postgres driver for migrate: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(sourceURL, p.db.DriverName(), driver)
	if err != nil {
		log.Logger.Error().Err(err).Str("source_url", sourceURL).Msg("Could not create migrate instance")
		return fmt.Errorf("could not create migrate instance with source '%s': %w", sourceURL, err)
	}

	log.Logger.Info().Msg("Applying migrations...")
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Logger.Error().Err(err).Msg("Failed to apply migrations")
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil {
		log.Logger.Warn().Err(err).Msg("Could not get migration version after applying")
	} else {
		log.Logger.Info().Uint("version", version).Bool("dirty", dirty).Msg("Migrations applied.")
	}

	log.Logger.Info().Msg("Database schema initialized/updated successfully via migrations.")
	return nil
}

func (p *PostgresDatastore) Close() error {
	if p.db != nil {
		log.Logger.Info().Msg("Closing PostgreSQL connection")
		return p.db.Close()
	}
	return nil
}

func redactDSN(dsnStr string, cfg config.Postgres) string {
	parsedDSN, err := url.Parse(dsnStr)
	if err != nil {
		log.Logger.Warn().Str("original_dsn", dsnStr).Msg("Failed to parse DSN for redaction, returning generic placeholder")
		return fmt.Sprintf("postgres://%s:*****@%s:%d/%s?sslmode=%s", cfg.Username, cfg.Address, cfg.Port, cfg.DBName, cfg.SSLMode)
	}

	if parsedDSN.User != nil {
		username := parsedDSN.User.Username()
		parsedDSN.User = url.UserPassword(username, "****")
	}

	return parsedDSN.String()
}
