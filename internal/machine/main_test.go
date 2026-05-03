package machine_test

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	migrate "github.com/adoublef/hey/internal/database/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func migrations(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()

	t.Cleanup(func() {
		ctx := context.Background()
		conn, err := pool.Acquire(ctx)
		ok(t, err)
		defer conn.Release()

		err = migrate.Down(ctx, conn.Conn())
		ok(t, err)
	})

	ctx := t.Context()
	conn, err := pool.Acquire(ctx)
	ok(t, err)
	defer conn.Release()

	err = migrate.Up(ctx, conn.Conn())
	ok(t, err)
}

var (
	dockerImagePostgres string
)

func init() {
	flag.StringVar(&dockerImagePostgres, "docker.image.postgres", "postgres:17-alpine", "postgres docker image")
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	if err := setup(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	if err := teardown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	} else if code > 0 {
		os.Exit(code)
	}
}

func equal[T comparable](t testing.TB, got, want T) {
	t.Helper()

	if got != want {
		t.Errorf("got %v; want %v", got, want)
	}
}

func ok(t testing.TB, errs ...error) {
	t.Helper()

	if err := cmp.Or(errs...); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func testDB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ctx := t.Context()

	dsn, err := postgresContainer.ConnectionString(ctx)
	ok(t, err)

	cfg, err := pgxpool.ParseConfig(dsn)
	ok(t, err)
	cfg.MaxConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	ok(t, err)

	t.Cleanup(pool.Close)
	return pool
}

var postgresContainer *postgres.PostgresContainer

func setup(ctx context.Context) (err error) {
	postgresContainer, err = postgres.Run(ctx, dockerImagePostgres, postgres.BasicWaitStrategies())
	return err
}

func teardown(ctx context.Context) error {
	return postgresContainer.Terminate(ctx)
}
