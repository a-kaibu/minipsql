package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yashikota/minipostgresql/internal/server"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "serve" {
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		host := fs.String("host", "127.0.0.1", "listen host")
		port := fs.Int("port", 15432, "listen port")
		var parseArgs []string
		if len(args) > 0 {
			parseArgs = args[1:]
		}
		if err := fs.Parse(parseArgs); err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		fmt.Fprintf(os.Stderr, "minipsql listening on %s:%d\n", *host, *port)
		err := server.New(server.Config{Host: *host, Port: *port}).ListenAndServe(ctx)
		if err == context.Canceled {
			return nil
		}
		return err
	}
	return fmt.Errorf("unknown command %q", args[0])
}
