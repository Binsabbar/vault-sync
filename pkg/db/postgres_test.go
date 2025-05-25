package db

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

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
		require.NoError(s.T(), err, "Should create datastore without error")
		defer store.Close()

		assert.NotNil(s.T(), store, "Expected store to be non-nil on successful connection")
		assert.NotNil(s.T(), store.DB, "Expected store.DB to be non-nil on successful connection")
		assert.Equal(s.T(), "pgx", store.DB.DriverName(), "Expected driver name to be 'pgx'")
	})

	s.T().Run("db connection failure returns error", func(t *testing.T) {
		badConfig := config.Postgres{
			Address:  "localhost",
			Port:     9999,
			Username: "wrong",
			Password: "wrong",
			DBName:   "wrongdb",
			SSLMode:  "disable",
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
		require.NoError(s.T(), err, "Should create datastore without error")
		defer store.Close()

		got := store.DB.Stats().MaxOpenConnections
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
		defer s.store.Close()

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
		defer s.store.Close()

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

// helper functions
// func (s *PostgresDatastoreTestSuite) truncateTable() {
// 	if s.store != nil || s.store.DB != nil {
// 		_, err := s.store.DB.Exec("TRUNCATE TABLE synced_secrets RESTART IDENTITY")
// 		require.NoError(s.T(), err, "Failed to truncate synced_secrets table")
// 	}
// }

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
