package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"sync"

	"github.com/yashikota/minipsql/internal/catalog"
	"github.com/yashikota/minipsql/internal/pgwire"
)

type Config struct {
	Host    string
	Port    int
	InitSQL string
}

type Server struct {
	cfg     Config
	handler *pgwire.Handler
}

func New(ctx context.Context, cfg Config) (*Server, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 15432
	}
	cat, err := catalog.New(ctx, cfg.InitSQL)
	if err != nil {
		return nil, err
	}
	handler := pgwire.NewHandler(cat)
	return &Server{cfg: cfg, handler: handler}, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port))
	if err != nil {
		return err
	}
	defer func() {
		_ = ln.Close()
	}()

	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				wg.Wait()
				return ctx.Err()
			}
			select {
			case errCh <- err:
			default:
			}
			wg.Wait()
			return <-errCh
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("panic in connection handler: %v\n%s", r, debug.Stack())
					_ = conn.Close()
				}
			}()
			s.handler.ServeConn(ctx, conn)
		}()
	}
}
