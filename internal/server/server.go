package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/yashikota/minipostgresql/internal/pgwire"
)

type Config struct {
	Host string
	Port int
}

type Server struct {
	cfg     Config
	handler *pgwire.Handler
}

func New(cfg Config) *Server {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 15432
	}
	return &Server{cfg: cfg, handler: pgwire.NewHandler()}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port))
	if err != nil {
		return err
	}
	defer ln.Close()

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
			s.handler.ServeConn(ctx, conn)
		}()
	}
}
