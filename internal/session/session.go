package session

import (
	"context"

	"github.com/yashikota/minipostgresql/internal/backend"
	"github.com/yashikota/minipostgresql/internal/compat"
)

type TxState byte

const (
	TxIdle   TxState = 'I'
	TxInTx   TxState = 'T'
	TxFailed TxState = 'E'
)

type Session struct {
	ID       int32
	Secret   int32
	Database string
	User     string
	TxState  TxState
	Params   map[string]string
	Backend  backend.Backend

	PreparedStatements map[string]*PreparedStatement
	Portals            map[string]*Portal
}

type PreparedStatement struct {
	Name          string
	SQL           string
	ParamTypeOIDs []uint32
	RewrittenSQL  string
}

type Portal struct {
	Name          string
	StatementName string
	Args          []backend.Value
	ResultFormats []int16
	Result        *backend.Result
	CommandTag    *backend.CommandTag
}

func New(id, secret int32, dbname, user string, b backend.Backend) *Session {
	if dbname == "" {
		dbname = "default"
	}
	if user == "" {
		user = "mini"
	}
	return &Session{
		ID:                 id,
		Secret:             secret,
		Database:           dbname,
		User:               user,
		TxState:            TxIdle,
		Params:             compat.DefaultParams(),
		Backend:            b,
		PreparedStatements: map[string]*PreparedStatement{},
		Portals:            map[string]*Portal{},
	}
}

func (s *Session) Execute(ctx context.Context, sql string, args []backend.Value) (*backend.Result, *backend.CommandTag, error) {
	if tx := compat.TxCommand(sql); tx != "" {
		switch tx {
		case "begin":
			if err := s.Backend.Begin(ctx); err != nil {
				s.TxState = TxFailed
				return nil, nil, err
			}
			s.TxState = TxInTx
			return nil, &backend.CommandTag{Tag: "BEGIN"}, nil
		case "commit":
			if err := s.Backend.Commit(ctx); err != nil {
				s.TxState = TxFailed
				return nil, nil, err
			}
			s.TxState = TxIdle
			return nil, &backend.CommandTag{Tag: "COMMIT"}, nil
		case "rollback":
			if err := s.Backend.Rollback(ctx); err != nil {
				s.TxState = TxFailed
				return nil, nil, err
			}
			s.TxState = TxIdle
			return nil, &backend.CommandTag{Tag: "ROLLBACK"}, nil
		}
	}

	info := compat.SessionInfo{Database: s.Database, User: s.User, Params: s.Params}
	if result, ok, err := compat.TryHandleBuiltin(ctx, s.Backend, sql, info); ok || err != nil {
		return result, nil, err
	}

	rewritten := compat.Rewrite(sql)
	if compat.IsQuery(rewritten) {
		result, err := s.Backend.Query(ctx, rewritten, args)
		return result, nil, err
	}
	tag, err := s.Backend.Exec(ctx, rewritten, args)
	return nil, tag, err
}
