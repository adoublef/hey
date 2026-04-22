package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/adoublef/hey/internal/flag/flagutil"
	"github.com/adoublef/hey/internal/log"
	"github.com/adoublef/hey/internal/net/http"
	"golang.org/x/sync/errgroup"
)

type serveCmd struct {
	port int
}

func (c *serveCmd) parse(args []string, getenv func(string) string) (err error) {
	var fs flag.FlagSet
	var (
		defaultPort = 3000
	)
	fs.IntVar(&c.port, "port", defaultPort, "http listening port")
	if err := fs.Parse(args); err != nil {
		return err
	} else if fs.NArg() > 0 {
		return fmt.Errorf("usage: serve [-port]: %w", flag.ErrHelp)
	}

	if s := getenv("PORT"); s != "" && c.port == defaultPort {
		c.port, err = flagutil.Atoi(s)
	}

	return err
}

func (c *serveCmd) run(ctx context.Context, stderr io.Writer) error {
	// ctx = log.WithLogger(ctx, slog.NewTextHandler(stderr, nil))
	// ^will rethink this API

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s := &http.Server{
		Addr:    ":" + strconv.Itoa(c.port),
		Handler: http.Handler(),
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
		log.Info(ctx, "http server running", slog.String("addr", s.Addr))
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
