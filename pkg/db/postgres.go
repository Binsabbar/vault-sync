package db

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/golang-migrate/migrate/v4"
	psqlmigrator "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"vault-sync/internal/config"
	"vault-sync/pkg/log"
)

var defaultHealthCheckPeriod = 1 * time.Minute

type PostgresDatastore struct {
	DB                  *sqlx.DB
	healthCheckInterval *time.Ticker
	stopHealthCheckCh   chan struct{}
	healthCheckDone     sync.WaitGroup
}

type PostgresConfig struct {
	config.Postgres
	MinimumConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

var migrationsPath = filepath.Join(".", "migrations", "postgres")

func NewPostgresDatastore(cfg config.Postgres) (*PostgresDatastore, error) {
	connectionString := buildPostgresDSN(cfg)
	redactedConnectionString := redactDSN(connectionString)

	log.Logger.Info().Str("dsn", redactedConnectionString).Msg("Attempting to connect to PostgreSQL")

	db, err := sqlx.Connect("pgx", connectionString)
	if err != nil {
		log.Logger.Error().Err(err).Str("dsn", redactedConnectionString).Msg("failed to connect to postgres")
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	defaultPoolConfig := defaultPoolConfig()
	defaultPoolConfig.Postgres = cfg
	setPoolConfig(defaultPoolConfig, db)

	if err := db.Ping(); err != nil {
		log.Logger.Error().Err(err).Str("dsn", redactedConnectionString).Msg("Failed to ping database")
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Logger.Info().Str("dsn", redactedConnectionString).Msg("Successfully connected to PostgreSQL")

	psqlDB := &PostgresDatastore{
		DB:                  db,
		healthCheckInterval: time.NewTicker(defaultHealthCheckPeriod),
		stopHealthCheckCh:   make(chan struct{}),
	}

	psqlDB.startHealthCheck()

	return psqlDB, psqlDB.initSchema()
}

func (p *PostgresDatastore) Close() error {
	if p.healthCheckInterval != nil {
		p.stopHealthCheckCh <- struct{}{}
		p.healthCheckDone.Wait()
	}
	log.Logger.Info().Msg("Waiting for PostgreSQL health check to finish...")
	if p.DB != nil {
		log.Logger.Info().Msg("Closing PostgreSQL connection")
		return p.DB.Close()
	}
	return nil
}

func redactDSN(dsnStr string) string {
	parsedDSN, _ := url.Parse(dsnStr)

	if parsedDSN.User != nil {
		username := parsedDSN.User.Username()
		parsedDSN.User = url.UserPassword(username, "xxxxx")
	}

	return parsedDSN.String()
}

func (p *PostgresDatastore) initSchema() error {
	log.Logger.Info().Str("migrations_path", migrationsPath).Msg("Initializing database schema via migrations...")
	driver, err := psqlmigrator.WithInstance(p.DB.DB, &psqlmigrator.Config{})
	if err != nil {
		log.Logger.Error().Err(err).Msg("Could not create postgres driver for migrate")
		return fmt.Errorf("could not create postgres driver for migrate: %w", err)
	}

	absMigrationsPath, err := filepath.Abs(migrationsPath)
	if err != nil {
		log.Logger.Error().Err(err).Str("path", migrationsPath).Msg("Failed to get absolute path for migrations")
		return fmt.Errorf("failed to get absolute path for migrations at %s: %w", migrationsPath, err)
	}
	sourceURL := fmt.Sprintf("file://%s", absMigrationsPath)

	m, err := migrate.NewWithDatabaseInstance(sourceURL, p.DB.DriverName(), driver)
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

func buildPostgresDSN(cfg config.Postgres) string {
	sslMode := cfg.SSLMode
	if sslMode == "" {
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

	return dsn.String()
}

func defaultPoolConfig() PostgresConfig {
	return PostgresConfig{
		MinimumConns:    5,
		ConnMaxLifetime: 15 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

func setPoolConfig(cfg PostgresConfig, db *sqlx.DB) {
	db.SetMaxIdleConns(cfg.MinimumConns)
	db.SetMaxOpenConns(cfg.MaxConnections)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	log.Logger.Debug().
		Int("max_open", cfg.MaxConnections).
		Int("max_idle", cfg.MinimumConns).
		Dur("max_lifetime", cfg.ConnMaxLifetime).
		Dur("max_idle_time", cfg.ConnMaxIdleTime).
		Msg("Configured PostgreSQL connection pool")
}

func (p *PostgresDatastore) startHealthCheck() {
	p.healthCheckDone.Add(1)
	go func() {
		for {
			select {
			case <-p.healthCheckInterval.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				err := p.DB.PingContext(ctx)
				if err != nil {
					log.Logger.Warn().Err(err).Msg("Database health check failed")
				}
			case <-p.stopHealthCheckCh:
				log.Logger.Info().Msg("Stopping database health check")
				p.healthCheckInterval.Stop()
				close(p.stopHealthCheckCh)
				log.Logger.Info().Msg("Stopped PostgreSQL health check")
				p.healthCheckInterval = nil
				p.stopHealthCheckCh = nil
				p.healthCheckDone.Done()
				return
			}
		}
	}()

	log.Logger.Info().Msg("Started database health check")
}
