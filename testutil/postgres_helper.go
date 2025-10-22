package testutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"vault-sync/internal/config"
	"vault-sync/pkg/log"
)

type PostgresHelper struct {
	Container *postgres.PostgresContainer
	Config    *config.Postgres
}

func NewPostgresContainer(t require.TestingT, ctx context.Context) (*PostgresHelper, error) {
	pm := getPortManager()
	randomPort, err := pm.reservePort()
	if err != nil {
		return nil, fmt.Errorf("failed to reserve port: %w", err)
	}

	return newPostgresContainerWithFixedPort(t, ctx, fmt.Sprintf("%d", randomPort))
}

func newPostgresContainerWithFixedPort(
	t require.TestingT,
	ctx context.Context,
	hostPort string,
) (*PostgresHelper, error) {
	dbUser := "testuser"
	dbPassword := "testpassword"
	dbName := "test_db"

	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		postgres.WithSQLDriver("pgx"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithStartupTimeout(1*time.Minute),
			wait.ForExposedPort().WithStartupTimeout(30*time.Second),
		),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").
				WithStartupTimeout(10*time.Second),
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
		if p.Container.IsRunning() {
			return p.Container.Stop(ctx, timeout)
		}
	}
	return nil
}

func (p *PostgresHelper) Start(ctx context.Context) error {
	if p.Container != nil {
		if !p.Container.IsRunning() {
			return p.Container.Start(ctx)
		}
	}
	return nil
}

func (p *PostgresHelper) ExecutePsqlCommand(ctx context.Context, sqlCommand string) (string, error) {
	command := fmt.Sprintf(`psql -t -U %s -d %s -c "%s"`, p.Config.Username, p.Config.DBName, sqlCommand)

	_, output, err := p.Container.Exec(ctx, []string{"sh", "-c", command}, exec.Multiplexed())
	if err != nil {
		return "", fmt.Errorf("failed to execute command %q in PSQL container: %w", command, err)
	}

	byteOutput, _ := io.ReadAll(output)
	if os.Getenv("DEBUG_TESTCONTAINERS") != "" {
		log.Logger.Info().Str("command", command).Msg("Executing PSQL command")
		log.Logger.Info().Str("output", string(byteOutput)).Msg("PSQL command output")
	}
	return string(byteOutput), nil
}
