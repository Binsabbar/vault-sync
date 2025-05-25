package testutil

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"vault-sync/internal/config"
)

type PostgresHelper struct {
	Container *postgres.PostgresContainer
	Config    *config.Postgres
}

func NewPostgresContainer(t require.TestingT, ctx context.Context) (*PostgresHelper, error) {
	dbUser := "testuser"
	dbPassword := "testpassword"
	dbName := "test_db"

	hostPort := "15432"
	pgContainer, err := postgres.Run(ctx,
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
			hostConfig.PortBindings = nat.PortMap{nat.Port("5432/tcp"): []nat.PortBinding{{HostPort: hostPort}}}
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL container: %w", err)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	portNat, err := pgContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	port, err := strconv.Atoi(portNat.Port())
	if err != nil {
		return nil, fmt.Errorf("failed to convert port to integer: %w", err)
	}

	pgConfig := &config.Postgres{
		Address:  host,
		Port:     port,
		Username: dbUser,
		Password: dbPassword,
		DBName:   dbName,
		SSLMode:  "disable",
	}

	require.NoError(t, err, "Failed to start PostgreSQL container")

	return &PostgresHelper{
		Container: pgContainer,
		Config:    pgConfig,
	}, nil
}

func (p *PostgresHelper) Terminate(ctx context.Context) error {
	if p.Container != nil {
		return p.Container.Terminate(ctx)
	}
	return nil
}

func (p *PostgresHelper) Stop(ctx context.Context, timeout *time.Duration) error {
	if p.Container != nil {
		return p.Container.Stop(ctx, timeout)
	}
	return nil
}

func (p *PostgresHelper) Start(ctx context.Context) error {
	if p.Container != nil {
		return p.Container.Start(ctx)
	}
	return nil
}
