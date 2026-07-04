package session

import (
	"context"

	"strings"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/compat"
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

	if tag, ok := s.tryNoopCommand(sql); ok {
		return nil, tag, nil
	}

	if err := checkUnsupportedDDL(sql); err != nil {
		return nil, nil, err
	}

	info := compat.SessionInfo{Database: s.Database, User: s.User, Params: s.Params}
	if result, ok, err := compat.TryHandleBuiltin(ctx, s.Backend, sql, info); ok || err != nil {
		return result, nil, err
	}

	rewritten := compat.Rewrite(sql)
	if compat.IsQuery(rewritten) || compat.HasReturning(rewritten) {
		result, err := s.Backend.Query(ctx, rewritten, args)
		return result, nil, err
	}
	tag, err := s.Backend.Exec(ctx, rewritten, args)
	return nil, tag, err
}

func checkUnsupportedDDL(sql string) error {
	q := strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	lower := strings.ToLower(q)

	unsupported := []struct {
		prefix string
		msg    string
	}{
		{"create operator", "CREATE OPERATOR is not supported"},
		{"alter operator", "ALTER OPERATOR is not supported"},
		{"drop operator", "DROP OPERATOR is not supported"},
		{"create function", "CREATE FUNCTION is not supported"},
		{"create or replace function", "CREATE FUNCTION is not supported"},
		{"drop function", "DROP FUNCTION is not supported"},
		{"alter function", "ALTER FUNCTION is not supported"},
		{"create procedure", "CREATE PROCEDURE is not supported"},
		{"create or replace procedure", "CREATE PROCEDURE is not supported"},
		{"drop procedure", "DROP PROCEDURE is not supported"},
		{"create aggregate", "CREATE AGGREGATE is not supported"},
		{"drop aggregate", "DROP AGGREGATE is not supported"},
		{"create type", "CREATE TYPE is not supported"},
		{"alter type", "ALTER TYPE is not supported"},
		{"drop type", "DROP TYPE is not supported"},
		{"create extension", "CREATE EXTENSION is not supported"},
		{"drop extension", "DROP EXTENSION is not supported"},
		{"create rule", "CREATE RULE is not supported"},
		{"drop rule", "DROP RULE is not supported"},
		{"create trigger", "CREATE TRIGGER is not supported"},
		{"drop trigger", "DROP TRIGGER is not supported"},
		{"create foreign", "CREATE FOREIGN is not supported"},
		{"alter foreign", "ALTER FOREIGN is not supported"},
		{"drop foreign", "DROP FOREIGN is not supported"},
		{"create role", "CREATE ROLE is not supported"},
		{"alter role", "ALTER ROLE is not supported"},
		{"drop role", "DROP ROLE is not supported"},
		{"create user", "CREATE USER is not supported"},
		{"alter user", "ALTER USER is not supported"},
		{"drop user", "DROP USER is not supported"},
		{"create schema", "CREATE SCHEMA is not supported"},
		{"drop schema", "DROP SCHEMA is not supported"},
		{"create domain", "CREATE DOMAIN is not supported"},
		{"alter domain", "ALTER DOMAIN is not supported"},
		{"drop domain", "DROP DOMAIN is not supported"},
		{"create conversion", "CREATE CONVERSION is not supported"},
		{"drop conversion", "DROP CONVERSION is not supported"},
		{"create statistics", "CREATE STATISTICS is not supported"},
		{"drop statistics", "DROP STATISTICS is not supported"},
		{"create language", "CREATE LANGUAGE is not supported"},
		{"drop language", "DROP LANGUAGE is not supported"},
		{"create cast", "CREATE CAST is not supported"},
		{"drop cast", "DROP CAST is not supported"},
		{"create collation", "CREATE COLLATION is not supported"},
		{"drop collation", "DROP COLLATION is not supported"},
		{"create access method", "CREATE ACCESS METHOD is not supported"},
		{"create event trigger", "CREATE EVENT TRIGGER is not supported"},
		{"create policy", "CREATE POLICY is not supported"},
		{"alter policy", "ALTER POLICY is not supported"},
		{"drop policy", "DROP POLICY is not supported"},
	}

	for _, u := range unsupported {
		if strings.HasPrefix(lower, u.prefix) {
			return &compat.PGError{Code: "0A000", Message: u.msg}
		}
	}

	if strings.HasPrefix(lower, "copy ") {
		return &compat.PGError{Code: "0A000", Message: "COPY is not supported"}
	}

	if strings.HasPrefix(lower, "do ") && strings.Contains(lower, "$$") {
		return &compat.PGError{Code: "0A000", Message: "DO blocks are not supported"}
	}

	return nil
}

func (s *Session) tryNoopCommand(sql string) (*backend.CommandTag, bool) {
	q := strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	lower := strings.ToLower(q)

	if strings.HasPrefix(lower, "set ") {
		parts := strings.SplitN(q[4:], "=", 2)
		if len(parts) == 1 {
			parts = strings.SplitN(q[4:], " to ", 2)
			if len(parts) == 1 {
				parts = strings.SplitN(strings.ToLower(q[4:]), " to ", 2)
			}
		}
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, "'\"")
			s.Params[name] = val
		}
		return &backend.CommandTag{Tag: "SET"}, true
	}

	if strings.HasPrefix(lower, "reset ") {
		return &backend.CommandTag{Tag: "RESET"}, true
	}

	switch {
	case strings.HasPrefix(lower, "discard "):
		return &backend.CommandTag{Tag: "DISCARD ALL"}, true
	case strings.HasPrefix(lower, "grant "):
		return &backend.CommandTag{Tag: "GRANT"}, true
	case strings.HasPrefix(lower, "revoke "):
		return &backend.CommandTag{Tag: "REVOKE"}, true
	case strings.HasPrefix(lower, "comment "):
		return &backend.CommandTag{Tag: "COMMENT"}, true
	case lower == "vacuum" || strings.HasPrefix(lower, "vacuum "):
		return &backend.CommandTag{Tag: "VACUUM"}, true
	case lower == "analyze" || strings.HasPrefix(lower, "analyze "):
		return &backend.CommandTag{Tag: "ANALYZE"}, true
	case strings.HasPrefix(lower, "lock "):
		return &backend.CommandTag{Tag: "LOCK TABLE"}, true
	case strings.HasPrefix(lower, "listen "):
		return &backend.CommandTag{Tag: "LISTEN"}, true
	case strings.HasPrefix(lower, "unlisten "):
		return &backend.CommandTag{Tag: "UNLISTEN"}, true
	case strings.HasPrefix(lower, "notify "):
		return &backend.CommandTag{Tag: "NOTIFY"}, true
	case strings.HasPrefix(lower, "deallocate "):
		return &backend.CommandTag{Tag: "DEALLOCATE"}, true
	case strings.HasPrefix(lower, "prepare "):
		return &backend.CommandTag{Tag: "PREPARE"}, true
	case strings.HasPrefix(lower, "execute "):
		return &backend.CommandTag{Tag: "EXECUTE"}, true
	case strings.HasPrefix(lower, "cluster"):
		return &backend.CommandTag{Tag: "CLUSTER"}, true
	case strings.HasPrefix(lower, "reindex"):
		return &backend.CommandTag{Tag: "REINDEX"}, true
	case strings.HasPrefix(lower, "checkpoint"):
		return &backend.CommandTag{Tag: "CHECKPOINT"}, true
	}

	return nil, false
}
