package postgres

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/tern/v2/migrate"
)

const defaultVersionTable string = "schema_version_non_default"

var defaultMigrator = &migrate.MigratorOptions{
	DisableTx: false,
}

func Up(ctx context.Context, conn *pgx.Conn, fs fs.FS) error {
	m, err := migrate.NewMigratorEx(ctx, conn, defaultVersionTable, defaultMigrator)
	if err != nil {
		return fmt.Errorf("failed to return migrator: %v", err)
	}
	err1 := m.LoadMigrations(fs)
	err2 := m.Migrate(ctx)
	if err := cmp.Or(err1, err2); err != nil {
		return fmt.Errorf("failed up migration: %v", err)
	}
	return nil
}

func Down(ctx context.Context, conn *pgx.Conn, fs fs.FS) error {
	m, err := migrate.NewMigratorEx(ctx, conn, defaultVersionTable, defaultMigrator)
	if err != nil {
		return fmt.Errorf("failed to return migrator: %v", err)
	}
	err1 := m.LoadMigrations(fs)
	err2 := m.MigrateTo(ctx, 0)
	if err := cmp.Or(err1, err2); err != nil {
		return fmt.Errorf("failed down migration: %v", err)
	}
	return nil
}
