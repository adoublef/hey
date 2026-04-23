package main

import (
	"cmp"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
)

type migrateCmd struct {
	dsn string // db.path=/path/to/directory
}

func (c *migrateCmd) parse(args []string, getenv func(string) string) (err error) {
	var fs flag.FlagSet
	var (
		defaultDSN = "" // current repo
	)
	fs.StringVar(&c.dsn, "dsn", defaultDSN, "database source name")
	if err := fs.Parse(args); err != nil {
		return err
	} else if fs.NArg() > 0 {
		return fmt.Errorf("usage: serve [-dsn]: %w", flag.ErrHelp)
	}

	// if s := getenv("DATABASE_URL"); s != "" && c.dsn == defaultDSN {
	c.dsn = cmp.Or(getenv("DATABASE_URL"), c.dsn)
	// }

	return nil
}

func (c *migrateCmd) run(ctx context.Context, stderr io.Writer) error {
	db, err := sql.Open("sqlite3", c.dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire: %v", err)
	}
	defer conn.Close()

	// Err() should work
	return conn.QueryRowContext(ctx, "SELECT 1").Scan(new(int))
}
