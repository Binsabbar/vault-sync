package datastore

import (
	"fmt"
	"path/filepath"

	"net/url"
	"vault-sync/internal/config"
	"vault-sync/internal/log"

	"github.com/golang-migrate/migrate/v4"
	psqlmigrator "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type PostgresDatastore struct {
	db                 *sqlx.DB
	preparedStatements *PostgresPreparedStatements
}

var (
	migrationsPath = filepath.Join("..", "..", "migrations", "postgres")
)

type PostgresPreparedStatements struct {
	selectSyncedSecretByPrimaryKey *sqlx.NamedStmt
	selectAllSyncedSecrets         *sqlx.NamedStmt
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
	pd := &PostgresDatastore{db: db}
	err = pd.initSchema()
	if err != nil {
		db.Close()
		return nil, err
	}
	pd.initPreparedStatements()
	return pd, nil
}

func (p *PostgresDatastore) Close() error {
	preparedStatements := []*sqlx.NamedStmt{
		p.preparedStatements.selectSyncedSecretByPrimaryKey,
		p.preparedStatements.selectAllSyncedSecrets,
	}
	if p.db != nil {
		log.Logger.Info().Msg("Closing PostgreSQL connection")
		for _, stmt := range preparedStatements {
			if stmt != nil {
				log.Logger.Info().Msg("Closing prepared statement")
				if err := stmt.Close(); err != nil {
					log.Logger.Error().Err(err).Msg("Failed to close prepared statement")
				}
			}
		}
		return p.db.Close()
	}
	return nil
}

func (p *PostgresDatastore) GetSyncedSecret(backend, path, destinationCluster string) (*SyncedSecret, error) {
	args := map[string]interface{}{
		"secret_backend":      backend,
		"secret_path":         path,
		"destination_cluster": destinationCluster,
	}
	var secret SyncedSecret

	err := p.preparedStatements.selectSyncedSecretByPrimaryKey.Get(&secret, args)
	if err != nil {
		log.Logger.Error().Err(err).Str("args", fmt.Sprintf("%s,%s,%s", backend, path, destinationCluster)).Msg("Failed to get synced secret")
		return nil, fmt.Errorf("failed to get synced secret: %w", err)
	}
	log.Logger.Info().Str("args", fmt.Sprintf("%s,%s,%s", backend, path, destinationCluster)).Msg("Successfully retrieved synced secret")
	return &secret, nil
}

func redactDSN(dsnStr string, cfg config.Postgres) string {
	parsedDSN, _ := url.Parse(dsnStr)

	if parsedDSN.User != nil {
		username := parsedDSN.User.Username()
		parsedDSN.User = url.UserPassword(username, "xxxxx")
	}

	return parsedDSN.String()
}

func (p *PostgresDatastore) GetSyncedSecrets() ([]SyncedSecret, error) {
	var secrets []SyncedSecret
	stmt := p.preparedStatements.selectAllSyncedSecrets
	err := stmt.Select(&secrets, struct{}{})
	if err != nil {
		log.Logger.Error().Err(err).Msg("Failed to get all synced secrets")
		return nil, fmt.Errorf("failed to get all synced secrets: %w", err)
	}
	return secrets, nil
}

func (p *PostgresDatastore) initSchema() error {
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

func (p *PostgresDatastore) initPreparedStatements() error {
	log.Logger.Info().Msg("Initializing prepared statements for PostgreSQL")
	stmtByPK, err := p.db.PrepareNamed(`
        SELECT * 
				FROM synced_secrets
        WHERE 
					secret_backend = :secret_backend AND secret_path = :secret_path AND destination_cluster = :destination_cluster
        LIMIT 1`)
	if err != nil {
		return fmt.Errorf("failed to prepare selectSyncedSecretByPrimaryKey: %w", err)
	}

	stmtAll, err := p.db.PrepareNamed(`
        SELECT * FROM synced_secrets ORDER BY secret_backend, secret_path, destination_cluster`)
	if err != nil {
		stmtByPK.Close()
		return fmt.Errorf("failed to prepare selectAllSyncedSecrets: %w", err)
	}

	p.preparedStatements = &PostgresPreparedStatements{
		selectSyncedSecretByPrimaryKey: stmtByPK,
		selectAllSyncedSecrets:         stmtAll,
	}

	return nil
}
