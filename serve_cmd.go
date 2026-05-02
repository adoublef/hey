package main

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/adoublef/hey/internal/cbz"
	"github.com/adoublef/hey/internal/eve"
	"github.com/adoublef/hey/internal/flag/flagutil"
	"github.com/adoublef/hey/internal/net/http"
	"golang.org/x/sync/errgroup"
)

type serveCmd struct {
	port int    // port=8000
	dsn  string // db.path=/path/to/directory
}

func (c *serveCmd) parse(args []string, getenv func(string) string) (err error) {
	var fs flag.FlagSet
	var (
		defaultPort = 3000
		// sqlitePath is the simpliest way to test this
		defaultDSN = "" // current repo
	)
	fs.IntVar(&c.port, "port", defaultPort, "http listening port")
	fs.StringVar(&c.dsn, "dsn", defaultDSN, "database source name")
	if err := fs.Parse(args); err != nil {
		return err
	} else if fs.NArg() > 0 {
		return fmt.Errorf("usage: serve [-port|-dsn]: %w", flag.ErrHelp)
	}

	var err1, err2 error
	if s := getenv("PORT"); s != "" && c.port == defaultPort {
		c.port, err1 = flagutil.Atoi(s)
	}
	// if s := getenv("DATABASE_URL"); s != "" && c.dsn == defaultDSN {
	c.dsn = cmp.Or(getenv("DATABASE_URL"), c.dsn)
	// }

	return cmp.Or(err1, err2)
}

func (c *serveCmd) run(ctx context.Context, stderr io.Writer) error {
	db, err := sql.Open("sqlite3", c.dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s := &http.Server{
		Addr:    ":" + strconv.Itoa(c.port),
		Handler: http.Handler(db, &eve.Client{C: http.DefaultClient}, &cbz.Client{C: http.DefaultClient}),
		// creating a new context on every rquest can work
		// but would htis be very wasteful?
		BaseContext: func(l net.Listener) context.Context { return ctx },
	}
	s.RegisterOnShutdown(cancel)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		if err = s.ListenAndServe(); http.IsServeClosed(err) {
			return nil
		}
		return
	})
	g.Go(func() (err error) {
		<-ctx.Done()
		ctx = context.WithoutCancel(ctx)

		ctx, cancel := context.WithTimeout(ctx, time.Second*60)
		defer cancel()

		if err = s.Shutdown(ctx); err != nil {
			err = errors.Join(err, s.Close())
		}
		return
	})
	return g.Wait()
}
