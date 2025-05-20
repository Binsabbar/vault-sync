package datastore

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"vault-sync/internal/config"
)

const dbSchema = `
CREATE TABLE IF NOT EXISTS synced_secrets (
    id SERIAL PRIMARY KEY,
    secret_path TEXT NOT NULL,
    source_version INTEGER NOT NULL,
    destination_cluster TEXT NOT NULL,
    destination_version INTEGER NOT NULL,
    last_sync_attempt TIMESTAMPTZ NOT NULL,
    last_sync_success TIMESTAMPTZ,
    status TEXT NOT NULL,
    error_message TEXT,
    UNIQUE (secret_path, destination_cluster)
);
`

type PostgresDatastoreTestSuite struct {
	suite.Suite
	pgContainer *postgres.PostgresContainer
	store       *PostgresDatastore
	pgConfig    config.Postgres
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
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithStartupTimeout(1*time.Minute),
		),
	)

	require.NoError(s.T(), err, "Failed to start PostgreSQL container")

	host, err := s.pgContainer.Host(ctx)
	portNat, err := s.pgContainer.MappedPort(ctx, "5432/tcp")
	port, err := strconv.Atoi(portNat.Port())

	s.pgConfig = config.Postgres{
		Address:  host,
		Port:     port,
		Username: dbUser,
		Password: dbPassword,
		DBName:   dbName,
		SSLMode:  "disable",
	}

	s.store, err = NewPostgresDatastore(s.pgConfig)
	require.NoError(s.T(), err, "Failed to create datastore")

	migrationsPath := filepath.Join("..", "..", "migrations", "postgres")
	err = s.store.InitSchema(migrationsPath)
	require.NoError(s.T(), err, "Failed to apply database migrations")
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

func (s *PostgresDatastoreTestSuite) BeforeTest(suiteName, testName string) {
	// Truncate table before each test to ensure isolation
	_, err := s.store.db.Exec("TRUNCATE TABLE synced_secrets RESTART IDENTITY")
	require.NoError(s.T(), err, "Failed to truncate synced_secrets table")
}
