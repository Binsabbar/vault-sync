package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // this is required to register the pgx driver with database/sql
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"github.com/golang-migrate/migrate/v4"
	psqlmigrator "github.com/golang-migrate/migrate/v4/database/postgres"

	"vault-sync/internal/config"
	"vault-sync/pkg/db/migrations"
	"vault-sync/pkg/log"
)

//nolint:gochecknoglobals
var defaultHealthCheckPeriod = 1 * time.Minute

type PostgresDatastore struct {
	DB                  *sqlx.DB
	migrationSource     migrations.MigrationSource
	healthCheckInterval *time.Ticker
	stopHealthCheckCh   chan struct{}
	healthCheckDone     sync.WaitGroup
	logger              zerolog.Logger
}

type PostgresConfig struct {
	*config.Postgres

	MinimumConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func NewPostgresDatastore(
	cfg *config.Postgres,
	migrationSource migrations.MigrationSource,
) (*PostgresDatastore, error) {
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

	if pingErr := db.Ping(); pingErr != nil {
		log.Logger.Error().Err(pingErr).Str("dsn", redactedConnectionString).Msg("Failed to ping database")
		return nil, fmt.Errorf("failed to ping database: %w", pingErr)
	}

	log.Logger.Info().Str("dsn", redactedConnectionString).Msg("Successfully connected to PostgreSQL")

	psqlDB := &PostgresDatastore{
		DB:                  db,
		migrationSource:     migrationSource,
		healthCheckInterval: time.NewTicker(defaultHealthCheckPeriod),
		stopHealthCheckCh:   make(chan struct{}),
		logger: log.Logger.With().
			Str("component", "postgres_datastore").
			Logger(),
	}

	psqlDB.startHealthCheck()

	return psqlDB, psqlDB.initSchema()
}

func (p *PostgresDatastore) Close() error {
	if p.healthCheckInterval != nil && p.stopHealthCheckCh != nil {
		select {
		case p.stopHealthCheckCh <- struct{}{}:
			p.logger.Info().Msg("Waiting for PostgreSQL health check to finish...")
			p.healthCheckDone.Wait()
		default:
		}
	}
	if p.DB != nil {
		p.logger.Info().Msg("Closing PostgreSQL connection")
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
	p.logger.Info().Msg("Initializing database schema via embedded migrations...")
	migrationSource := p.migrationSource
	d, err := migrationSource.GetSourceDriver()
	if err != nil {
		return err
	}

	driver, err := psqlmigrator.WithInstance(p.DB.DB, &psqlmigrator.Config{})
	if err != nil {
		p.logger.Error().Err(err).Msg("Could not create postgres driver for migrate")
		return fmt.Errorf("could not create postgres driver for migrate: %w", err)
	}

	m, err := migrate.NewWithInstance(migrationSource.GetSourceType(), d, p.DB.DriverName(), driver)
	if err != nil {
		p.logger.Error().Err(err).Msg("Could not create migrate instance")
		return fmt.Errorf("could not create migrate instance: %w", err)
	}

	p.logger.Info().Msg("Applying migrations...")
	if upErr := m.Up(); upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		p.logger.Error().Err(upErr).Msg("Failed to apply migrations")
		return fmt.Errorf("failed to apply migrations: %w", upErr)
	}

	version, dirty, err := m.Version()
	if err != nil {
		p.logger.Warn().Err(err).Msg("Could not get migration version after applying")
	} else {
		p.logger.Info().Uint("version", version).Bool("dirty", dirty).Msg("Migrations applied.")
	}

	p.logger.Info().Msg("Database schema initialized/updated successfully via migrations.")
	return nil
}

func buildPostgresDSN(cfg *config.Postgres) string {
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

//nolint:mnd
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

//nolint:mnd
func (p *PostgresDatastore) startHealthCheck() {
	p.healthCheckDone.Add(1)
	go func() {
		for {
			select {
			case <-p.healthCheckInterval.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := p.DB.PingContext(ctx)
				if err != nil {
					p.logger.Warn().Err(err).Msg("Database health check failed")
				}
				cancel()
			case <-p.stopHealthCheckCh:
				p.logger.Info().Msg("Stopping database health check")
				p.healthCheckInterval.Stop()
				close(p.stopHealthCheckCh)
				p.logger.Info().Msg("Stopped PostgreSQL health check")
				p.healthCheckInterval = nil
				p.stopHealthCheckCh = nil
				p.healthCheckDone.Done()
				return
			}
		}
	}()

	p.logger.Info().Msg("Started database health check")
}
