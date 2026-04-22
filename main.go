package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background())
	defer cancel()

	var (
		args   = os.Args[1:]
		getenv = os.Getenv
		stderr = os.Stderr
	)

	if err := run(ctx, args, getenv, stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) error {
	var name string
	if len(args) > 0 {
		name, args = args[0], args[1:]
	}

	type cmd interface {
		parse(args []string, getenv func(string) string) error
		run(ctx context.Context, stderr io.Writer) error
	}

	var cc = map[string]cmd{
		"serve": new(serveCmd),
	}
	c, ok := cc[name]
	if !ok {
		return fmt.Errorf("unknown command %q: %w", name, flag.ErrHelp)
	} else if err := c.parse(args, getenv); err != nil {
		return err
	}

	return c.run(ctx, stderr)
}
