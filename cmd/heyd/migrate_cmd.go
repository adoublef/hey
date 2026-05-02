package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"

	migrate "github.com/adoublef/hey/internal/database/postgres"
	"github.com/adoublef/hey/internal/machine"
	"github.com/jackc/pgx/v5"
)

type migrateCmd struct {
	dsn  string // db.path=/path/to/directory
	down bool
}

func (c *migrateCmd) parse(args []string, getenv func(string) string) (err error) {
	var fs flag.FlagSet
	var (
		defaultDSN = "" // current repo
	)
	fs.StringVar(&c.dsn, "dsn", defaultDSN, "database source name")
	fs.BoolVar(&c.down, "down", false, "drop table state")
	if err := fs.Parse(args); err != nil {
		return err
	} else if fs.NArg() > 0 {
		return fmt.Errorf("usage: serve [-dsn|-down]: %w", flag.ErrHelp)
	}

	// if s := getenv("DATABASE_URL"); s != "" && c.dsn == defaultDSN {
	c.dsn = cmp.Or(getenv("DATABASE_URL"), c.dsn)
	// }

	return nil
}

func (c *migrateCmd) run(ctx context.Context, stderr io.Writer) error {
	conn, err := pgx.Connect(ctx, c.dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer conn.Close(context.Background())

	if c.down {
		return migrate.Down(ctx, conn, machine.FS)
	}
	return migrate.Up(ctx, conn, machine.FS)
}
