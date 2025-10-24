package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4/source"
	"github.com/stretchr/testify/suite"

	"vault-sync/internal/config"
	"vault-sync/pkg/db/migrations"
	"vault-sync/testutil"
)

type PostgresDatastoreTestSuite struct {
	suite.Suite
	pgHelper *testutil.PostgresHelper
}

type badMigrationSource struct{}

func (b *badMigrationSource) GetSourceType() string {
	return "iofs"
}
func (b *badMigrationSource) GetSourceDriver() (source.Driver, error) {
	return nil, fmt.Errorf("failed to create migration source: simulated error")
}

func (suite *PostgresDatastoreTestSuite) SetupTest() {
	ctx := context.Background()
	var err error

	suite.pgHelper, err = testutil.NewPostgresContainer(suite.T(), ctx)
	suite.NoError(err, "Failed to create Postgres test container")
}

func (suite *PostgresDatastoreTestSuite) TearDownTest() {
	ctx := context.Background()
	if suite.pgHelper != nil {
		err := suite.pgHelper.Terminate(ctx)
		if err != nil {
			log.Printf("Error terminating container: %v", err)
		}
	}
}

func TestPostgresDatastoreSuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}
	suite.Run(t, new(PostgresDatastoreTestSuite))
}

var postgresMigrator = migrations.NewPostgresMigration()

func (suite *PostgresDatastoreTestSuite) TestNewPostgresDatastore() {

	suite.Run("successful connection to postgres", func() {
		store, err := NewPostgresDatastore(suite.pgHelper.Config, postgresMigrator)
		suite.NoError(err, "Should create datastore without error")

		suite.NotNil(store, "Expected store to be non-nil on successful connection")
		suite.NotNil(store.DB, "Expected store.DB to be non-nil on successful connection")
		suite.Equal("pgx", store.DB.DriverName(), "Expected driver name to be 'pgx'")
	})

	suite.Run("db connection failure returns error", func() {
		badConfig := &config.Postgres{
			Address:  "localhost",
			Port:     9999,
			Username: "wrong",
			Password: "wrong",
			DBName:   "wrongdb",
		}

		store, err := NewPostgresDatastore(badConfig, postgresMigrator)

		suite.Nil(store, "Expected store to be nil on connection failure")
		suite.Error(err, "Expected error when connecting to invalid postgres instance")
		suite.Contains(err.Error(), "failed to connect to postgres", "Error message should indicate connection failure")
	})

	suite.Run("set maxConnection when it is configured", func() {
		cfg := testutil.CopyStruct(suite.pgHelper.Config)
		cfg.MaxConnections = 5
		store, err := NewPostgresDatastore(cfg, postgresMigrator)
		suite.NoError(err, "Should create datastore without error")

		got := store.DB.Stats().MaxOpenConnections

		suite.Equal(cfg.MaxConnections, got, "MaxOpenConnections should match config.MaxConnections")
	})
}

type testColumn struct {
	DataType   string
	IsNullable string
}

func (suite *PostgresDatastoreTestSuite) TestInitSchema_VerifySTableStructure() {

	suite.Run("verifies synced_secrets table structure", func() {
		expectedColumns := map[string]testColumn{
			"secret_backend":      {"text", "NO"},
			"secret_path":         {"text", "NO"},
			"destination_cluster": {"text", "NO"},
			"source_version":      {"integer", "NO"},
			"destination_version": {"integer", "NO"},
			"last_sync_attempt":   {"timestamp with time zone", "NO"},
			"last_sync_success":   {"timestamp with time zone", "YES"},
			"status":              {"text", "NO"},
			"error_message":       {"text", "YES"},
		}

		store, err := NewPostgresDatastore(suite.pgHelper.Config, postgresMigrator)
		suite.NoError(err, "Should create datastore without error")

		actualColumns := getColumns(store, "public", "synced_secrets")

		suite.Len(actualColumns, len(expectedColumns), "Number of columns does not match expected")

		for col, exp := range expectedColumns {
			act, ok := actualColumns[col]
			suite.True(ok, "Expected column '%s' not found", col)
			suite.Equal(exp.DataType, act.DataType, "Data type mismatch for column '%s'", col)
			suite.True(strings.EqualFold(exp.IsNullable, act.IsNullable), "Nullability mismatch for column '%s'", col)
		}
	})

	suite.Run("verifies primary key constraint on id column", func() {
		store, err := NewPostgresDatastore(suite.pgHelper.Config, postgresMigrator)
		suite.NoError(err, "Should create datastore without error")

		var pkColumns = getPrimaryKeyColumns(store, "public", "synced_secrets")

		suite.Equal([]string{"secret_backend", "secret_path", "destination_cluster"}, pkColumns, "PRIMARY KEY should be on 'id'")
	})

	suite.Run("returns error if migration source is broken", func() {
		// Custom migration source that always fails
		badSource := &badMigrationSource{}

		_, err := NewPostgresDatastore(suite.pgHelper.Config, badSource)

		suite.Error(err)
		suite.Contains(err.Error(), "failed to create migration source")
	})

	suite.Run("returns error if migration fails", func() {
		store, err := NewPostgresDatastore(suite.pgHelper.Config, migrations.NewPostgresMigration())
		suite.NoError(err)

		suite.pgHelper.Stop(context.Background(), nil)
		defer suite.pgHelper.Start(context.Background())

		err = store.initSchema()
		suite.Error(err)
	})

}

func (suite *PostgresDatastoreTestSuite) TestHealthCheck() {
	suite.Run("health check continues after database temporary outage", func() {
		shortInterval := 300 * time.Millisecond
		originalHealthCheckPeriod := defaultHealthCheckPeriod
		defaultHealthCheckPeriod = shortInterval
		defer func() { defaultHealthCheckPeriod = originalHealthCheckPeriod }()

		store, err := NewPostgresDatastore(suite.pgHelper.Config, postgresMigrator)
		suite.NoError(err)

		// Let it run a few cycles
		time.Sleep(shortInterval * 3)

		ctx := context.Background()
		err = suite.pgHelper.Stop(ctx, &shortInterval)

		// Wait for a few health check cycles during the outage
		time.Sleep(shortInterval * 3)

		// Resume the container
		err = suite.pgHelper.Start(ctx)
		suite.NoError(err)

		suite.Eventually(func() bool {
			var count int
			err = store.DB.Get(&count, "SELECT 1")
			return err == nil && count == 1
		}, time.Second*10, time.Millisecond*100, "Health check should still be running after recovery")
	})

	suite.Run("it stops healthcheck when DB is closed", func() {
		store, err := NewPostgresDatastore(suite.pgHelper.Config, postgresMigrator)

		suite.NoError(err, "Should create datastore without error")

		store.Close()

		suite.Nil(store.healthCheckInterval)
		suite.Nil(store.stopHealthCheckCh)
	})
}

func getColumns(store *PostgresDatastore, schema string, table string) map[string]testColumn {
	query := `
				SELECT column_name, data_type, is_nullable
				FROM information_schema.columns
				WHERE table_schema = $1
					AND table_name   = $2
				ORDER BY ordinal_position;
		`
	rows, _ := store.DB.Queryx(query, schema, table)
	defer rows.Close()

	cols := make(map[string]testColumn)
	for rows.Next() {
		var name, dataType, isNullable string
		rows.Scan(&name, &dataType, &isNullable)
		cols[name] = testColumn{dataType, isNullable}
	}
	return cols
}

func getConstraintColumns(store *PostgresDatastore, constraintName string, schema string, table string) []string {
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
	store.DB.Select(&columns, query, constraintName, schema, table)
	return columns
}

func getPrimaryKeyColumns(store *PostgresDatastore, schema string, table string) []string {
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
	store.DB.Select(&columns, query, schema, table)
	return columns
}
