package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/yashikota/minipsql/internal/server"
)

func Run(args []string, version string) error {
	return run(args, version, os.Stderr)
}

func run(args []string, version string, stderr io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "-v", "--version", "version":
			_, _ = fmt.Fprintln(stderr, version)
			return nil
		}
	}

	if len(args) == 0 || args[0] == "serve" {
		return runServe(args, stderr)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func runServe(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	host := fs.String("host", "127.0.0.1", "listen host")
	port := fs.Int("port", 15432, "listen port")
	initPath := fs.String("init", "", "SQL file to execute for each in-memory database")
	var parseArgs []string
	if len(args) > 0 {
		parseArgs = args[1:]
	}
	if err := fs.Parse(parseArgs); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	initSQL := ""
	if *initPath != "" {
		data, err := os.ReadFile(*initPath)
		if err != nil {
			return err
		}
		initSQL = string(data)
	}
	_, _ = fmt.Fprintf(stderr, "minipsql listening on %s:%d\n", *host, *port)
	err := server.New(server.Config{Host: *host, Port: *port, InitSQL: initSQL}).ListenAndServe(ctx)
	if err == context.Canceled {
		return nil
	}
	return err
}
