package db

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"vault-sync/internal/config"
)

type PostgresDatastoreTestSuite struct {
	suite.Suite
	pgContainer *postgres.PostgresContainer
	store       *PostgresDatastore
	pgConfig    *config.Postgres
}

type testColumn struct {
	DataType   string
	IsNullable string
}

func TestPostgresDatastoreSuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}
	suite.Run(t, new(PostgresDatastoreTestSuite))
}

func (s *PostgresDatastoreTestSuite) SetupSuite() {
	ctx := context.Background()
	var err error

	dbUser := "testuser"
	dbPassword := "testpassword"
	dbName := "test_db"
	s.pgContainer, err = postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		postgres.WithSQLDriver("pgx"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithStartupTimeout(1*time.Minute),
			wait.ForExposedPort().WithStartupTimeout(1*time.Minute),
		),
		testcontainers.WithHostConfigModifier(func(hostConfig *container.HostConfig) {
			hostConfig.PortBindings = nat.PortMap{nat.Port("5432/tcp"): []nat.PortBinding{{HostPort: "15432"}}}
		}),
	)

	require.NoError(s.T(), err, "Failed to start PostgreSQL container")

	host, err := s.pgContainer.Host(ctx)
	portNat, err := s.pgContainer.MappedPort(ctx, "5432/tcp")
	port, err := strconv.Atoi(portNat.Port())

	s.pgConfig = &config.Postgres{
		Address:  host,
		Port:     port,
		Username: dbUser,
		Password: dbPassword,
		DBName:   dbName,
		SSLMode:  "disable",
	}
}

func (s *PostgresDatastoreTestSuite) TearDownSuite() {
	ctx := context.Background()
	if s.store != nil {
		err := s.store.Close()
		if err != nil {
			log.Printf("Error closing datastore: %v", err)
		}
	}
	if s.pgContainer != nil {
		err := s.pgContainer.Terminate(ctx)
		if err != nil {
			log.Printf("Error terminating container: %v", err)
		}
	}
}

func (s *PostgresDatastoreTestSuite) TestNewPostgresDatastore() {

	s.T().Run("successful connection to postgres", func(t *testing.T) {
		store, err := NewPostgresDatastore(*s.pgConfig)
		s.store = store
		require.NoError(s.T(), err, "Should create datastore without error")

		assert.NotNil(s.T(), s.store, "Expected store to be non-nil on successful connection")
		assert.NotNil(s.T(), s.store.DB, "Expected store.DB to be non-nil on successful connection")
		assert.Equal(s.T(), "pgx", s.store.DB.DriverName(), "Expected driver name to be 'pgx'")
	})

	s.T().Run("db connection failure returns error", func(t *testing.T) {
		badConfig := config.Postgres{
			Address:  "localhost",
			Port:     9999,
			Username: "wrong",
			Password: "wrong",
			DBName:   "wrongdb",
		}

		store, err := NewPostgresDatastore(badConfig)

		assert.Nil(s.T(), store, "Expected store to be nil on connection failure")
		assert.Error(s.T(), err, "Expected error when connecting to invalid postgres instance")
		assert.Contains(s.T(), err.Error(), "failed to connect to postgres", "Error message should indicate connection failure")
	})

	s.T().Run("set maxConnection when it is configured", func(t *testing.T) {
		cfg := *s.pgConfig
		cfg.MaxConnections = 5
		store, err := NewPostgresDatastore(cfg)
		s.store = store
		require.NoError(s.T(), err, "Should create datastore without error")

		got := s.store.DB.Stats().MaxOpenConnections

		assert.Equal(s.T(), cfg.MaxConnections, got, "MaxOpenConnections should match config.MaxConnections")
	})
}

func (s *PostgresDatastoreTestSuite) TestInitSchema_VerifySTableStructure() {

	s.T().Run("verifies synced_secrets table structure", func(t *testing.T) {
		expectedColumns := map[string]testColumn{
			"secret_backend":      {"text", "NO"},
			"secret_path":         {"text", "NO"},
			"destination_cluster": {"text", "NO"},
			"source_version":      {"integer", "NO"},
			"destination_version": {"integer", "YES"},
			"last_sync_attempt":   {"timestamp with time zone", "NO"},
			"last_sync_success":   {"timestamp with time zone", "YES"},
			"status":              {"text", "NO"},
			"error_message":       {"text", "YES"},
		}

		store, err := NewPostgresDatastore(*s.pgConfig)
		require.NoError(s.T(), err, "Should create datastore without error")
		s.store = store

		actualColumns := s.getColumns("public", "synced_secrets")

		assert.Len(s.T(), actualColumns, len(expectedColumns), "Number of columns does not match expected")

		for col, exp := range expectedColumns {
			act, ok := actualColumns[col]
			assert.True(s.T(), ok, "Expected column '%s' not found", col)
			assert.Equal(s.T(), exp.DataType, act.DataType, "Data type mismatch for column '%s'", col)
			assert.True(s.T(), strings.EqualFold(exp.IsNullable, act.IsNullable), "Nullability mismatch for column '%s'", col)
		}
	})

	s.T().Run("verifies primary key constraint on id column", func(t *testing.T) {
		store, err := NewPostgresDatastore(*s.pgConfig)
		require.NoError(s.T(), err, "Should create datastore without error")
		s.store = store

		var pkColumns = s.getPrimaryKeyColumns("public", "synced_secrets")

		assert.Equal(s.T(), []string{"secret_backend", "secret_path", "destination_cluster"}, pkColumns, "PRIMARY KEY should be on 'id'")
	})

	s.T().Run("returns error if migrate instance creation fails", func(t *testing.T) {
		oldPath := migrationsPath
		defer func() { migrationsPath = oldPath }()
		migrationsPath = "/non/existent/path"

		_, err := NewPostgresDatastore(*s.pgConfig)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not create migrate instance with source")
	})

	s.T().Run("returns error if migration fails", func(t *testing.T) {
		oldPath := migrationsPath
		defer func() { migrationsPath = oldPath }()
		migrationsPath = t.TempDir()

		_, err := NewPostgresDatastore(*s.pgConfig)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to apply migrations")
	})
}

func (s *PostgresDatastoreTestSuite) TestHealthCheck() {
	s.T().Run("health check continues after database temporary outage", func(t *testing.T) {
		shortInterval := 300 * time.Millisecond
		originalHealthCheckPeriod := defaultHealthCheckPeriod
		defaultHealthCheckPeriod = shortInterval
		defer func() { defaultHealthCheckPeriod = originalHealthCheckPeriod }()

		config := *s.pgConfig
		store, err := NewPostgresDatastore(config)
		require.NoError(t, err)
		s.store = store

		// Let it run a few cycles
		time.Sleep(shortInterval * 3)

		// Pause the container to simulate a DB outage
		ctx := context.Background()
		err = s.pgContainer.Stop(ctx, &shortInterval)

		// Wait for a few health check cycles during the outage
		time.Sleep(shortInterval * 3)

		// Resume the container
		err = s.pgContainer.Start(ctx)
		require.NoError(t, err)

		// Wait for recovery
		time.Sleep(time.Second * 3)

		var count int
		err = store.DB.Get(&count, "SELECT 1")
		assert.NoError(t, err, "Database should be working after recovery")
	})

	s.T().Run("it stops helathcheck when DB is closed", func(t *testing.T) {
		config := *s.pgConfig
		store, err := NewPostgresDatastore(config)
		s.store = store
		require.NoError(t, err, "Should create datastore without error")

		s.store.Close()

		assert.Nil(t, s.store.healthCheckInterval)
		assert.Nil(t, s.store.stopHealthCheckCh)
	})
}

func (s *PostgresDatastoreTestSuite) getColumns(schema string, table string) map[string]testColumn {
	query := `
				SELECT column_name, data_type, is_nullable
				FROM information_schema.columns
				WHERE table_schema = $1
					AND table_name   = $2
				ORDER BY ordinal_position;
		`
	rows, _ := s.store.DB.Queryx(query, schema, table)
	defer rows.Close()

	cols := make(map[string]testColumn)
	for rows.Next() {
		var name, dataType, isNullable string
		assert.NoError(s.T(), rows.Scan(&name, &dataType, &isNullable))
		cols[name] = testColumn{dataType, isNullable}
	}
	return cols
}

func (s *PostgresDatastoreTestSuite) getConstraintColumns(constraintName string, schema string, table string) []string {
	query := `
        SELECT kcu.column_name
        FROM information_schema.table_constraints tc
        JOIN information_schema.key_column_usage kcu
          ON tc.constraint_name = kcu.constraint_name
          AND tc.table_schema = kcu.table_schema
        WHERE tc.constraint_type = $1
          AND tc.table_name = $3
          AND tc.table_schema = $2
        ORDER BY kcu.ordinal_position;
    `
	var columns []string
	s.store.DB.Select(&columns, query, constraintName, schema, table)
	return columns
}

func (s *PostgresDatastoreTestSuite) getPrimaryKeyColumns(schema string, table string) []string {
	query := `
        SELECT kcu.column_name
        FROM information_schema.table_constraints tc
        JOIN information_schema.key_column_usage kcu
          ON tc.constraint_name = kcu.constraint_name
          AND tc.table_schema = kcu.table_schema
        WHERE tc.constraint_type = 'PRIMARY KEY'
          AND tc.table_name = $2
          AND tc.table_schema = $1
        ORDER BY kcu.ordinal_position;
    `
	var columns []string
	s.store.DB.Select(&columns, query, schema, table)
	return columns
}
