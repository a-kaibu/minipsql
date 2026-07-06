package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/compat"
	pgcopy "github.com/yashikota/minipsql/internal/copy"
)

type TxState byte

const (
	TxIdle   TxState = 'I'
	TxInTx   TxState = 'T'
	TxFailed TxState = 'E'
)

// CatalogOperations is implemented by catalog.Catalog; defined here to avoid import cycles.
type CatalogOperations interface {
	CreateDatabase(ctx context.Context, name string) error
	DropDatabase(ctx context.Context, name string) error
}

type Session struct {
	ID       int32
	Secret   int32
	Database string
	User     string
	TxState  TxState
	Params   map[string]string
	Backend  backend.Backend
	Catalog  CatalogOperations

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

func New(id, secret int32, dbname, user string, b backend.Backend, cat CatalogOperations) *Session {
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
		Catalog:            cat,
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

	if tag, err := s.tryDatabaseCommand(ctx, sql); tag != nil || err != nil {
		return nil, tag, err
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

	return nil
}

func (s *Session) tryNoopCommand(sql string) (*backend.CommandTag, bool) {
	q := strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	lower := strings.ToLower(q)

	if strings.HasPrefix(lower, "set ") {
		var parts []string
		if idx := strings.Index(q[4:], "="); idx >= 0 {
			parts = []string{q[4 : 4+idx], q[4+idx+1:]}
		} else {
			lowerSuffix := strings.ToLower(q[4:])
			if idx := strings.Index(lowerSuffix, " to "); idx >= 0 {
				parts = []string{q[4 : 4+idx], q[4+idx+4:]}
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
	case lower == "do" || strings.HasPrefix(lower, "do "):
		return &backend.CommandTag{Tag: "DO"}, true
	case strings.HasPrefix(lower, "discard "):
		return &backend.CommandTag{Tag: "DISCARD ALL"}, true
	case strings.HasPrefix(lower, "create schema"):
		return &backend.CommandTag{Tag: "CREATE SCHEMA"}, true
	case strings.HasPrefix(lower, "drop schema"):
		return &backend.CommandTag{Tag: "DROP SCHEMA"}, true
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

func (s *Session) tryDatabaseCommand(ctx context.Context, sql string) (*backend.CommandTag, error) {
	q := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(sql, ";"), " "))
	lower := strings.ToLower(q)

	if strings.HasPrefix(lower, "create database ") {
		rest := q[len("create database "):]
		ifNotExists := false
		if strings.HasPrefix(strings.ToLower(rest), "if not exists ") {
			ifNotExists = true
			rest = rest[len("if not exists "):]
		}
		name := parseDatabaseName(rest)
		if s.Catalog == nil {
			return &backend.CommandTag{Tag: "CREATE DATABASE"}, nil
		}
		err := s.Catalog.CreateDatabase(ctx, name)
		if err != nil {
			if ifNotExists {
				if pg, ok := err.(*compat.PGError); ok && pg.Code == "42P04" {
					return &backend.CommandTag{Tag: "CREATE DATABASE"}, nil
				}
			}
			return nil, err
		}
		return &backend.CommandTag{Tag: "CREATE DATABASE"}, nil
	}

	if strings.HasPrefix(lower, "drop database ") {
		rest := q[len("drop database "):]
		ifExists := false
		if strings.HasPrefix(strings.ToLower(rest), "if exists ") {
			ifExists = true
			rest = rest[len("if exists "):]
		}
		name := parseDatabaseName(rest)
		if s.Catalog == nil {
			return &backend.CommandTag{Tag: "DROP DATABASE"}, nil
		}
		err := s.Catalog.DropDatabase(ctx, name)
		if err != nil {
			if ifExists {
				if pg, ok := err.(*compat.PGError); ok && pg.Code == "3D000" {
					return &backend.CommandTag{Tag: "DROP DATABASE"}, nil
				}
			}
			return nil, err
		}
		return &backend.CommandTag{Tag: "DROP DATABASE"}, nil
	}

	return nil, nil
}

func parseDatabaseName(s string) string {
	name := strings.TrimSpace(s)
	if strings.HasPrefix(name, "\"") {
		end := strings.Index(name[1:], "\"")
		if end >= 0 {
			return name[1 : end+1]
		}
	}
	// stop at first whitespace (strips trailing options like WITH OWNER …)
	if idx := strings.IndexAny(name, " \t\n\r"); idx >= 0 {
		name = name[:idx]
	}
	return strings.ToLower(name)
}

func (s *Session) ExecuteCopyFrom(ctx context.Context, stmt *pgcopy.Statement, data []byte) (*backend.CommandTag, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	if stmt.Header && len(lines) > 0 {
		lines = lines[1:]
	}

	// resolve columns
	columns := stmt.Columns
	if len(columns) == 0 {
		cols, err := s.Backend.TableColumns(ctx, stmt.TableName)
		if err != nil {
			return nil, fmt.Errorf("COPY: cannot resolve columns for %q: %w", stmt.TableName, err)
		}
		columns = cols
	}

	var rows [][]backend.Value
	for _, line := range lines {
		if line == "" {
			continue
		}
		var fields []string
		var eof bool
		var err error
		if stmt.Format == "csv" {
			fields, eof, err = pgcopy.ParseCSVRow(line, stmt.Delimiter, stmt.NullString, stmt.Quote, stmt.Escape)
		} else {
			fields, eof, err = pgcopy.ParseTextRow(line, stmt.Delimiter, stmt.NullString)
		}
		if err != nil {
			return nil, fmt.Errorf("COPY: parse error: %w", err)
		}
		if eof {
			break
		}
		row := make([]backend.Value, len(columns))
		for i := 0; i < len(columns); i++ {
			if i < len(fields) {
				f := fields[i]
				if pgcopy.IsNullMarker(f) {
					row[i] = backend.Value{Kind: backend.ValueNull}
				} else {
					row[i] = backend.Value{Kind: backend.ValueText, Value: f}
				}
			} else {
				row[i] = backend.Value{Kind: backend.ValueNull}
			}
		}
		rows = append(rows, row)
	}

	n, err := s.Backend.BulkInsert(ctx, stmt.TableName, columns, rows)
	if err != nil {
		return nil, err
	}
	return &backend.CommandTag{Tag: fmt.Sprintf("COPY %d", n)}, nil
}
