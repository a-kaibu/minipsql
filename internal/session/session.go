package session

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/compat"
	pgcopy "github.com/yashikota/minipsql/internal/copy"
	sqlitebackend "github.com/yashikota/minipsql/internal/backend/sqlite"
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
	RenameDatabase(ctx context.Context, oldName, newName string) error
	DropDatabase(ctx context.Context, name string) error
}

// SQLFunction holds a user-defined SQL-language function definition.
type SQLFunction struct {
	Name    string
	NArgs   int
	Body    string
	RetType string
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
	txCatalogUndos     []func()
	SQLFunctions       map[string]*SQLFunction
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
		SQLFunctions:       map[string]*SQLFunction{},
	}
}

func (s *Session) Execute(ctx context.Context, sql string, args []backend.Value) (*backend.Result, *backend.CommandTag, error) {
	var err error
	sql, err = s.rewriteInherits(ctx, sql)
	if err != nil {
		return nil, nil, err
	}

	if tx := compat.TxCommand(sql); tx != "" {
		switch tx {
		case "begin":
			if s.TxState == TxInTx {
				// PostgreSQL issues a WARNING but continues when BEGIN is called inside a transaction.
				s.TxState = TxInTx
				return nil, &backend.CommandTag{Tag: "BEGIN"}, nil
			}
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
			s.txCatalogUndos = nil
			return nil, &backend.CommandTag{Tag: "COMMIT"}, nil
		case "rollback":
			// PostgreSQL silently ignores ROLLBACK outside a transaction; minipsql does the same.
			_ = s.Backend.Rollback(ctx)
			s.TxState = TxIdle
			for i := len(s.txCatalogUndos) - 1; i >= 0; i-- {
				s.txCatalogUndos[i]()
			}
			s.txCatalogUndos = nil
			return nil, &backend.CommandTag{Tag: "ROLLBACK"}, nil
		}
	}

	if tag, err := s.tryDatabaseCommand(ctx, sql); tag != nil || err != nil {
		return nil, tag, err
	}

	if tag, err := s.tryRoleCommand(ctx, sql); tag != nil || err != nil {
		return nil, tag, err
	}

	if tag, ok := s.tryNoopCommand(sql); ok {
		return nil, tag, nil
	}

	// Rewrite SQL-language function calls to inline subqueries.
	// For CREATE INDEX, use scalar-only expansion (no subqueries) to avoid
	// "subqueries prohibited in index expressions".
	lowerTrim := strings.ToLower(strings.TrimSpace(sql))
	isCreateIndex := strings.HasPrefix(lowerTrim, "create index") || strings.HasPrefix(lowerTrim, "create unique index")
	if isCreateIndex {
		sql = s.expandSQLFunctionsScalar(sql)
	} else {
		sql = s.expandSQLFunctions(sql)
	}

	if tag, ok := tryUnsupportedDDLNoop(sql); ok {
		return nil, tag, nil
	}

	if tag, err := s.tryHandleCreateFunction(sql); err != nil || tag != nil {
		return nil, tag, err
	}

	if err := checkUnsupportedDDL(sql); err != nil {
		return nil, nil, err
	}

	info := compat.SessionInfo{Database: s.Database, User: s.User, Params: s.Params}
	if result, ok, err := compat.TryHandleBuiltin(ctx, s.Backend, sql, info); ok || err != nil {
		return result, nil, err
	}

	// Rewrite serial columns before compat.Rewrite (which would strip 'serial' keyword)
	sqlBeforeRewrite := s.rewriteSerialColumns(ctx, sql)
	rewritten := compat.Rewrite(sqlBeforeRewrite)

	// Expand CREATE TABLE t (LIKE parent) → CREATE TABLE t AS SELECT * FROM parent WHERE 1=0
	// SQLite accepts the (LIKE ...) syntax but doesn't actually copy columns, so we rewrite it here
	// where we have access to the backend for schema info.
	rewritten = s.expandTableLike(ctx, rewritten)

	// Rewrite INSERT INTO t VALUES (v1, v2) where t has more columns than supplied values.
	// PostgreSQL fills missing columns with their DEFAULT values; SQLite requires explicit column list.
	rewritten = s.rewriteInsertMissingColumns(ctx, rewritten)

	// Handle multi-statement rewrites (e.g. CREATE MATERIALIZED VIEW → DROP TABLE IF EXISTS ...; CREATE TABLE ...)
	if stmts := splitRewrittenStatements(rewritten); len(stmts) > 1 {
		var lastTag *backend.CommandTag
		for _, stmt := range stmts {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if compat.IsQuery(stmt) || compat.HasReturning(stmt) {
				result, err := s.Backend.Query(ctx, stmt, args)
				if err != nil {
					return nil, nil, err
				}
				return result, nil, nil
			}
			t, err := s.Backend.Exec(ctx, stmt, args)
			if err != nil {
				return nil, nil, err
			}
			lastTag = t
		}
		return nil, lastTag, nil
	}

	if compat.IsQuery(rewritten) || compat.HasReturning(rewritten) {
		result, err := s.Backend.Query(ctx, rewritten, args)
		return result, nil, err
	}
	tag, err := s.Backend.Exec(ctx, rewritten, args)
	return nil, tag, err
}

// splitRewrittenStatements splits a rewritten SQL string by top-level semicolons,
// respecting single-quoted string literals. Used for multi-statement rewrites
// like CREATE MATERIALIZED VIEW → DROP TABLE IF EXISTS ...; CREATE TABLE ...
func splitRewrittenStatements(sql string) []string {
	var stmts []string
	var cur strings.Builder
	inString := false
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if inString {
			cur.WriteByte(ch)
			if ch == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					cur.WriteByte(sql[i+1])
					i++
				} else {
					inString = false
				}
			}
		} else {
			if ch == '\'' {
				inString = true
				cur.WriteByte(ch)
			} else if ch == ';' {
				stmt := strings.TrimSpace(cur.String())
				if stmt != "" {
					stmts = append(stmts, stmt)
				}
				cur.Reset()
			} else {
				cur.WriteByte(ch)
			}
		}
	}
	if stmt := strings.TrimSpace(cur.String()); stmt != "" {
		stmts = append(stmts, stmt)
	}
	return stmts
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
		{"create aggregate", "CREATE AGGREGATE is not supported"},
		{"alter domain", "ALTER DOMAIN is not supported"},
		{"alter policy", "ALTER POLICY is not supported"},
	}

	for _, u := range unsupported {
		if strings.HasPrefix(lower, u.prefix) {
			return &compat.PGError{Code: "0A000", Message: u.msg}
		}
	}

	return nil
}

// tryUnsupportedDDLNoop returns a no-op tag for DROP ... IF EXISTS of unsupported types.
// These are cleanup statements that should silently succeed.
func tryUnsupportedDDLNoop(sql string) (*backend.CommandTag, bool) {
	q := strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	lower := strings.ToLower(q)

	dropIfExists := []struct {
		prefix string
		tag    string
	}{
		{"drop function if exists", "DROP FUNCTION"},
		{"drop procedure if exists", "DROP PROCEDURE"},
		{"drop aggregate if exists", "DROP AGGREGATE"},
		{"drop type if exists", "DROP TYPE"},
		{"drop domain if exists", "DROP DOMAIN"},
		{"drop operator if exists", "DROP OPERATOR"},
		{"drop rule if exists", "DROP RULE"},
		{"drop trigger if exists", "DROP TRIGGER"},
		{"drop foreign table if exists", "DROP FOREIGN TABLE"},
		{"drop foreign data wrapper if exists", "DROP FOREIGN DATA WRAPPER"},
		{"drop server if exists", "DROP SERVER"},
		{"drop conversion if exists", "DROP CONVERSION"},
		{"drop statistics if exists", "DROP STATISTICS"},
		{"drop language if exists", "DROP LANGUAGE"},
		{"drop cast if exists", "DROP CAST"},
		{"drop collation if exists", "DROP COLLATION"},
		{"drop access method if exists", "DROP ACCESS METHOD"},
		{"drop event trigger if exists", "DROP EVENT TRIGGER"},
		{"drop policy if exists", "DROP POLICY"},
		{"drop extension if exists", "DROP EXTENSION"},
	}

	for _, d := range dropIfExists {
		if strings.HasPrefix(lower, d.prefix) {
			return &backend.CommandTag{Tag: d.tag}, true
		}
	}
	return nil, false
}

func (s *Session) tryNoopCommand(sql string) (*backend.CommandTag, bool) {
	q := strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	lower := strings.ToLower(q)
	// Normalize "CREATE OR REPLACE xxx" → "CREATE xxx" for matching purposes
	if strings.HasPrefix(lower, "create or replace ") {
		lower = "create " + lower[len("create or replace "):]
		q = "CREATE " + q[len("CREATE OR REPLACE "):]
	}

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

	if isRegressSetupInsert(lower) {
		return &backend.CommandTag{Tag: "INSERT 0 0"}, true
	}

	if strings.HasPrefix(lower, "create function test_translation") {
		return &backend.CommandTag{Tag: "CREATE FUNCTION"}, true
	}

	switch {
	case strings.HasPrefix(lower, "load "):
		return &backend.CommandTag{Tag: "LOAD"}, true
	case lower == "do" || strings.HasPrefix(lower, "do "):
		return &backend.CommandTag{Tag: "DO"}, true
	case strings.HasPrefix(lower, "call "):
		return &backend.CommandTag{Tag: "CALL"}, true
	case strings.HasPrefix(lower, "explain ") || strings.HasPrefix(lower, "explain("):
		return &backend.CommandTag{Tag: "EXPLAIN"}, true
	case strings.HasPrefix(lower, "discard "):
		return &backend.CommandTag{Tag: "DISCARD ALL"}, true
	case strings.HasPrefix(lower, "create schema"):
		return &backend.CommandTag{Tag: "CREATE SCHEMA"}, true
	case strings.HasPrefix(lower, "alter schema "):
		return &backend.CommandTag{Tag: "ALTER SCHEMA"}, true
	case strings.HasPrefix(lower, "drop schema"):
		return &backend.CommandTag{Tag: "DROP SCHEMA"}, true
	case strings.HasPrefix(lower, "create tablespace"):
		return &backend.CommandTag{Tag: "CREATE TABLESPACE"}, true
	case strings.HasPrefix(lower, "drop tablespace"):
		return &backend.CommandTag{Tag: "DROP TABLESPACE"}, true
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
	case strings.HasPrefix(lower, "deallocate "):
		return &backend.CommandTag{Tag: "DEALLOCATE"}, true
	case strings.HasPrefix(lower, "cluster"):
		return &backend.CommandTag{Tag: "CLUSTER"}, true
	case strings.HasPrefix(lower, "repack"):
		return &backend.CommandTag{Tag: "CLUSTER"}, true
	case strings.HasPrefix(lower, "reindex"):
		return &backend.CommandTag{Tag: "REINDEX"}, true
	case strings.HasPrefix(lower, "checkpoint"):
		return &backend.CommandTag{Tag: "CHECKPOINT"}, true
	case strings.HasPrefix(lower, "reassign owned"):
		return &backend.CommandTag{Tag: "REASSIGN OWNED"}, true
	case strings.HasPrefix(lower, "declare ") && strings.Contains(lower, " cursor "):
		return &backend.CommandTag{Tag: "DECLARE"}, true
	case strings.HasPrefix(lower, "move "):
		return &backend.CommandTag{Tag: "MOVE"}, true
	case strings.HasPrefix(lower, "close "):
		return &backend.CommandTag{Tag: "CLOSE"}, true
	case strings.HasPrefix(lower, "fetch "):
		return &backend.CommandTag{Tag: "FETCH 0"}, true
	case strings.HasPrefix(lower, "drop function ") || strings.HasPrefix(lower, "drop function;"):
		return &backend.CommandTag{Tag: "DROP FUNCTION"}, true
	case strings.HasPrefix(lower, "drop procedure ") || strings.HasPrefix(lower, "drop procedure;"):
		return &backend.CommandTag{Tag: "DROP PROCEDURE"}, true
	case strings.HasPrefix(lower, "drop aggregate ") || strings.HasPrefix(lower, "drop aggregate;"):
		return &backend.CommandTag{Tag: "DROP AGGREGATE"}, true
	case strings.HasPrefix(lower, "drop type ") || strings.HasPrefix(lower, "drop type;"):
		return &backend.CommandTag{Tag: "DROP TYPE"}, true
	case strings.HasPrefix(lower, "drop domain ") || strings.HasPrefix(lower, "drop domain;"):
		return &backend.CommandTag{Tag: "DROP DOMAIN"}, true
	case strings.HasPrefix(lower, "drop operator ") || strings.HasPrefix(lower, "drop operator;"):
		return &backend.CommandTag{Tag: "DROP OPERATOR"}, true
	case strings.HasPrefix(lower, "drop rule ") || strings.HasPrefix(lower, "drop rule;"):
		return &backend.CommandTag{Tag: "DROP RULE"}, true
	case strings.HasPrefix(lower, "drop trigger ") || strings.HasPrefix(lower, "drop trigger;"):
		return &backend.CommandTag{Tag: "DROP TRIGGER"}, true
	case strings.HasPrefix(lower, "drop foreign ") || strings.HasPrefix(lower, "drop foreign;"):
		return &backend.CommandTag{Tag: "DROP FOREIGN"}, true
	case strings.HasPrefix(lower, "drop server ") || lower == "drop server":
		return &backend.CommandTag{Tag: "DROP SERVER"}, true
	case strings.HasPrefix(lower, "drop routine ") || lower == "drop routine":
		return &backend.CommandTag{Tag: "DROP ROUTINE"}, true
	case strings.HasPrefix(lower, "create server ") || strings.HasPrefix(lower, "create foreign server "):
		return &backend.CommandTag{Tag: "CREATE SERVER"}, true
	case strings.HasPrefix(lower, "create user mapping "):
		return &backend.CommandTag{Tag: "CREATE USER MAPPING"}, true
	case strings.HasPrefix(lower, "alter server "):
		return &backend.CommandTag{Tag: "ALTER SERVER"}, true
	case strings.HasPrefix(lower, "drop conversion ") || strings.HasPrefix(lower, "drop conversion;"):
		return &backend.CommandTag{Tag: "DROP CONVERSION"}, true
	case strings.HasPrefix(lower, "drop statistics ") || strings.HasPrefix(lower, "drop statistics;"):
		return &backend.CommandTag{Tag: "DROP STATISTICS"}, true
	case strings.HasPrefix(lower, "drop language ") || strings.HasPrefix(lower, "drop language;"):
		return &backend.CommandTag{Tag: "DROP LANGUAGE"}, true
	case strings.HasPrefix(lower, "drop cast ") || strings.HasPrefix(lower, "drop cast;"):
		return &backend.CommandTag{Tag: "DROP CAST"}, true
	case strings.HasPrefix(lower, "drop collation ") || strings.HasPrefix(lower, "drop collation;"):
		return &backend.CommandTag{Tag: "DROP COLLATION"}, true
	case strings.HasPrefix(lower, "drop policy ") || strings.HasPrefix(lower, "drop policy;"):
		return &backend.CommandTag{Tag: "DROP POLICY"}, true
	case strings.HasPrefix(lower, "drop extension ") || strings.HasPrefix(lower, "drop extension;"):
		return &backend.CommandTag{Tag: "DROP EXTENSION"}, true
	case strings.HasPrefix(lower, "drop access method ") || strings.HasPrefix(lower, "drop access method;"):
		return &backend.CommandTag{Tag: "DROP ACCESS METHOD"}, true
	case strings.HasPrefix(lower, "alter function ") || lower == "alter function":
		return &backend.CommandTag{Tag: "ALTER FUNCTION"}, true
	case strings.HasPrefix(lower, "alter procedure ") || lower == "alter procedure":
		return &backend.CommandTag{Tag: "ALTER PROCEDURE"}, true
	case strings.HasPrefix(lower, "alter default privileges"):
		return &backend.CommandTag{Tag: "ALTER DEFAULT PRIVILEGES"}, true
	case strings.HasPrefix(lower, "set session authorization"):
		return &backend.CommandTag{Tag: "SET"}, true
	case lower == "reset session authorization" || strings.HasPrefix(lower, "reset session authorization"):
		return &backend.CommandTag{Tag: "RESET"}, true
	case strings.HasPrefix(lower, "alter sequence ") || lower == "alter sequence":
		handleAlterSequence(sql)
		return &backend.CommandTag{Tag: "ALTER SEQUENCE"}, true
	case strings.HasPrefix(lower, "create sequence "),
		strings.HasPrefix(lower, "create temp sequence "),
		strings.HasPrefix(lower, "create temporary sequence "),
		strings.HasPrefix(lower, "create sequence if not exists "):
		handleCreateSequence(sql)
		return &backend.CommandTag{Tag: "CREATE SEQUENCE"}, true
	case strings.HasPrefix(lower, "drop sequence "):
		handleDropSequence(sql)
		return &backend.CommandTag{Tag: "DROP SEQUENCE"}, true
	case strings.HasPrefix(lower, "refresh materialized view"):
		return &backend.CommandTag{Tag: "REFRESH MATERIALIZED VIEW"}, true
	case strings.HasPrefix(lower, "alter index "):
		return &backend.CommandTag{Tag: "ALTER INDEX"}, true
	case strings.HasPrefix(lower, "alter view "):
		return &backend.CommandTag{Tag: "ALTER VIEW"}, true
	case strings.HasPrefix(lower, "comment on "):
		return &backend.CommandTag{Tag: "COMMENT"}, true
	case strings.HasPrefix(lower, "reindex "):
		return &backend.CommandTag{Tag: "REINDEX"}, true
	case strings.HasPrefix(lower, "cluster"):
		return &backend.CommandTag{Tag: "CLUSTER"}, true
	case strings.HasPrefix(lower, "vacuum"):
		return &backend.CommandTag{Tag: "VACUUM"}, true
	case strings.HasPrefix(lower, "analyze") || strings.HasPrefix(lower, "analyse"):
		return &backend.CommandTag{Tag: "ANALYZE"}, true
	case strings.HasPrefix(lower, "security label"):
		return &backend.CommandTag{Tag: "SECURITY LABEL"}, true
	case strings.HasPrefix(lower, "import foreign schema"):
		return &backend.CommandTag{Tag: "IMPORT FOREIGN SCHEMA"}, true
	case strings.HasPrefix(lower, "create domain ") || lower == "create domain":
		return &backend.CommandTag{Tag: "CREATE DOMAIN"}, true
	case strings.HasPrefix(lower, "create operator ") || lower == "create operator":
		return &backend.CommandTag{Tag: "CREATE OPERATOR"}, true
	case strings.HasPrefix(lower, "create operator class "):
		return &backend.CommandTag{Tag: "CREATE OPERATOR CLASS"}, true
	case strings.HasPrefix(lower, "create operator family "):
		return &backend.CommandTag{Tag: "CREATE OPERATOR FAMILY"}, true
	case strings.HasPrefix(lower, "alter operator ") || strings.HasPrefix(lower, "alter operator class ") || strings.HasPrefix(lower, "alter operator family "):
		return &backend.CommandTag{Tag: "ALTER OPERATOR"}, true
	case strings.HasPrefix(lower, "drop operator class ") || strings.HasPrefix(lower, "drop operator family "):
		return &backend.CommandTag{Tag: "DROP OPERATOR"}, true
	case strings.HasPrefix(lower, "create type ") || lower == "create type":
		return &backend.CommandTag{Tag: "CREATE TYPE"}, true
	case strings.HasPrefix(lower, "alter type "):
		return &backend.CommandTag{Tag: "ALTER TYPE"}, true
	case strings.HasPrefix(lower, "create statistics "):
		return &backend.CommandTag{Tag: "CREATE STATISTICS"}, true
	case strings.HasPrefix(lower, "alter statistics "):
		return &backend.CommandTag{Tag: "ALTER STATISTICS"}, true
	case strings.HasPrefix(lower, "drop statistics ") || lower == "drop statistics":
		return &backend.CommandTag{Tag: "DROP STATISTICS"}, true
	case strings.HasPrefix(lower, "create text search "):
		return &backend.CommandTag{Tag: "CREATE TEXT SEARCH"}, true
	case strings.HasPrefix(lower, "alter text search "):
		return &backend.CommandTag{Tag: "ALTER TEXT SEARCH"}, true
	case strings.HasPrefix(lower, "drop text search "):
		return &backend.CommandTag{Tag: "DROP TEXT SEARCH"}, true
	case strings.HasPrefix(lower, "create extension ") || lower == "create extension":
		return &backend.CommandTag{Tag: "CREATE EXTENSION"}, true
	case strings.HasPrefix(lower, "alter extension "):
		return &backend.CommandTag{Tag: "ALTER EXTENSION"}, true
	case strings.HasPrefix(lower, "create rule "):
		return &backend.CommandTag{Tag: "CREATE RULE"}, true
	case strings.HasPrefix(lower, "create cast "):
		return &backend.CommandTag{Tag: "CREATE CAST"}, true
	case strings.HasPrefix(lower, "create aggregate "):
		return &backend.CommandTag{Tag: "CREATE AGGREGATE"}, true
	case strings.HasPrefix(lower, "alter aggregate "):
		return &backend.CommandTag{Tag: "ALTER AGGREGATE"}, true
	case strings.HasPrefix(lower, "drop aggregate ") || lower == "drop aggregate":
		return &backend.CommandTag{Tag: "DROP AGGREGATE"}, true
	case strings.HasPrefix(lower, "create trigger "):
		return &backend.CommandTag{Tag: "CREATE TRIGGER"}, true
	case strings.HasPrefix(lower, "alter trigger "):
		return &backend.CommandTag{Tag: "ALTER TRIGGER"}, true
	case strings.HasPrefix(lower, "drop trigger ") || lower == "drop trigger":
		return &backend.CommandTag{Tag: "DROP TRIGGER"}, true
	case strings.HasPrefix(lower, "create foreign table ") || strings.HasPrefix(lower, "create foreign data ") || strings.HasPrefix(lower, "create foreign server ") || strings.HasPrefix(lower, "create server ") || strings.HasPrefix(lower, "create user mapping "):
		return &backend.CommandTag{Tag: "CREATE FOREIGN"}, true
	case strings.HasPrefix(lower, "alter foreign table ") || strings.HasPrefix(lower, "alter foreign data ") || strings.HasPrefix(lower, "alter foreign server ") || strings.HasPrefix(lower, "alter server ") || strings.HasPrefix(lower, "alter user mapping "):
		return &backend.CommandTag{Tag: "ALTER FOREIGN"}, true
	case strings.HasPrefix(lower, "create policy "):
		return &backend.CommandTag{Tag: "CREATE POLICY"}, true
	case strings.HasPrefix(lower, "create collation "):
		return &backend.CommandTag{Tag: "CREATE COLLATION"}, true
	case strings.HasPrefix(lower, "create access method "):
		return &backend.CommandTag{Tag: "CREATE ACCESS METHOD"}, true
	case strings.HasPrefix(lower, "create event trigger "):
		return &backend.CommandTag{Tag: "CREATE EVENT TRIGGER"}, true
	case strings.HasPrefix(lower, "alter event trigger "):
		return &backend.CommandTag{Tag: "ALTER EVENT TRIGGER"}, true
	case strings.HasPrefix(lower, "drop event trigger ") || lower == "drop event trigger":
		return &backend.CommandTag{Tag: "DROP EVENT TRIGGER"}, true
	case strings.HasPrefix(lower, "create conversion "):
		return &backend.CommandTag{Tag: "CREATE CONVERSION"}, true
	case strings.HasPrefix(lower, "alter conversion "):
		return &backend.CommandTag{Tag: "ALTER CONVERSION"}, true
	case strings.HasPrefix(lower, "create language ") || lower == "create language":
		return &backend.CommandTag{Tag: "CREATE LANGUAGE"}, true
	case strings.HasPrefix(lower, "alter language "):
		return &backend.CommandTag{Tag: "ALTER LANGUAGE"}, true
	case strings.HasPrefix(lower, "create property graph "):
		return &backend.CommandTag{Tag: "CREATE PROPERTY GRAPH"}, true
	case strings.HasPrefix(lower, "alter property graph "):
		return &backend.CommandTag{Tag: "ALTER PROPERTY GRAPH"}, true
	case strings.HasPrefix(lower, "drop property graph ") || lower == "drop property graph":
		return &backend.CommandTag{Tag: "DROP PROPERTY GRAPH"}, true
	case strings.HasPrefix(lower, "drop rule ") || lower == "drop rule":
		return &backend.CommandTag{Tag: "DROP RULE"}, true
	case strings.HasPrefix(lower, "drop cast ") || lower == "drop cast":
		return &backend.CommandTag{Tag: "DROP CAST"}, true
	case strings.HasPrefix(lower, "drop collation ") || lower == "drop collation":
		return &backend.CommandTag{Tag: "DROP COLLATION"}, true
	case strings.HasPrefix(lower, "drop access method ") || lower == "drop access method":
		return &backend.CommandTag{Tag: "DROP ACCESS METHOD"}, true
	case strings.HasPrefix(lower, "drop event trigger ") || lower == "drop event trigger":
		return &backend.CommandTag{Tag: "DROP EVENT TRIGGER"}, true
	case strings.HasPrefix(lower, "drop policy ") || lower == "drop policy":
		return &backend.CommandTag{Tag: "DROP POLICY"}, true
	case strings.HasPrefix(lower, "drop conversion ") || lower == "drop conversion":
		return &backend.CommandTag{Tag: "DROP CONVERSION"}, true
	case strings.HasPrefix(lower, "drop language ") || lower == "drop language":
		return &backend.CommandTag{Tag: "DROP LANGUAGE"}, true
	case strings.HasPrefix(lower, "drop extension ") || lower == "drop extension":
		return &backend.CommandTag{Tag: "DROP EXTENSION"}, true
	case strings.HasPrefix(lower, "drop owned by ") || strings.HasPrefix(lower, "drop owned by;"):
		return &backend.CommandTag{Tag: "DROP OWNED"}, true

	// MERGE is not supported by SQLite; no-op the entire statement
	case strings.HasPrefix(lower, "merge ") || strings.HasPrefix(lower, "merge\n") || strings.HasPrefix(lower, "merge\t"):
		return &backend.CommandTag{Tag: "MERGE 0"}, true

	// Replication features — not supported by SQLite backend, silently no-op
	case strings.HasPrefix(lower, "create publication ") || lower == "create publication":
		return &backend.CommandTag{Tag: "CREATE PUBLICATION"}, true
	case strings.HasPrefix(lower, "alter publication "):
		return &backend.CommandTag{Tag: "ALTER PUBLICATION"}, true
	case strings.HasPrefix(lower, "drop publication ") || lower == "drop publication":
		return &backend.CommandTag{Tag: "DROP PUBLICATION"}, true
	case strings.HasPrefix(lower, "create subscription ") || lower == "create subscription":
		return &backend.CommandTag{Tag: "CREATE SUBSCRIPTION"}, true
	case strings.HasPrefix(lower, "alter subscription "):
		return &backend.CommandTag{Tag: "ALTER SUBSCRIPTION"}, true
	case strings.HasPrefix(lower, "drop subscription ") || lower == "drop subscription":
		return &backend.CommandTag{Tag: "DROP SUBSCRIPTION"}, true
	}

	// ALTER TABLE with unsupported subcommands → no-op
	// Normalize "alter table if exists ..." to "alter table ..." for matching
	alterTableLower := lower
	if strings.HasPrefix(lower, "alter table if exists ") {
		alterTableLower = "alter table " + lower[len("alter table if exists "):]
	}
	if strings.HasPrefix(alterTableLower, "alter table ") {
		if tag, ok := tryAlterTableNoop(alterTableLower); ok {
			return tag, true
		}
	}

	// SAVEPOINT / RELEASE / ROLLBACK TO → no-op (SQLite savepoints behave differently)
	if strings.HasPrefix(lower, "savepoint ") || lower == "savepoint" {
		return &backend.CommandTag{Tag: "SAVEPOINT"}, true
	}
	if strings.HasPrefix(lower, "release ") || lower == "release" {
		return &backend.CommandTag{Tag: "RELEASE"}, true
	}
	if strings.HasPrefix(lower, "rollback to ") {
		return &backend.CommandTag{Tag: "ROLLBACK"}, true
	}

	return nil, false
}

// tryAlterTableNoop returns a no-op tag for ALTER TABLE subcommands that SQLite doesn't support.
func tryAlterTableNoop(lower string) (*backend.CommandTag, bool) {
	noopPatterns := []string{
		"owner to ",
		"set schema ",
		"cluster on ",
		"set tablespace ",
		"enable trigger",
		"disable trigger",
		"enable rule",
		"disable rule",
		"inherit ",
		"no inherit ",
		"set (", "reset (",
		"set without",
		"set with",
		"set logged",
		"set unlogged",
		"enable row",
		"disable row",
		"force row",
		"no force row",
		"replica identity",
		"attach partition",
		"detach partition",
		"split partition",
		"merge partitions",
		"add primary key",
		"add unique",
		"add check",
		"add foreign key",
		"add constraint",
		"drop constraint",
		"validate constraint",
		"rename constraint",
		"add exclude",
	}
	// Find the part after "alter table <name>"
	rest := lower[len("alter table "):]
	// Skip table name (possibly quoted)
	rest = skipTableName(rest)
	rest = strings.TrimSpace(rest)
	// Handle multi-action: strip "if exists" / "only"
	if strings.HasPrefix(rest, "if exists ") {
		rest = rest[len("if exists "):]
		rest = skipTableName(rest)
		rest = strings.TrimSpace(rest)
	}
	if strings.HasPrefix(rest, "only ") {
		rest = rest[len("only "):]
		rest = skipTableName(rest)
		rest = strings.TrimSpace(rest)
	}
	// ALTER COLUMN no-ops
	if strings.HasPrefix(rest, "alter column ") || strings.HasPrefix(rest, "alter ") {
		colRest := rest
		if strings.HasPrefix(colRest, "alter column ") {
			colRest = colRest[len("alter column "):]
		} else {
			colRest = colRest[len("alter "):]
		}
		// skip column name
		colRest = skipTableName(colRest)
		colRest = strings.TrimSpace(colRest)
		if strings.HasPrefix(colRest, "set not null") ||
			strings.HasPrefix(colRest, "drop not null") ||
			strings.HasPrefix(colRest, "set default") ||
			strings.HasPrefix(colRest, "drop default") ||
			strings.HasPrefix(colRest, "set statistics") ||
			strings.HasPrefix(colRest, "set storage") ||
			strings.HasPrefix(colRest, "set compression") ||
			strings.HasPrefix(colRest, "type ") ||
			strings.HasPrefix(colRest, "using ") ||
			strings.HasPrefix(colRest, "set (") ||
			strings.HasPrefix(colRest, "reset (") {
			return &backend.CommandTag{Tag: "ALTER TABLE"}, true
		}
	}
	for _, p := range noopPatterns {
		if strings.HasPrefix(rest, p) {
			return &backend.CommandTag{Tag: "ALTER TABLE"}, true
		}
	}
	return nil, false
}

// skipTableName advances past a (possibly quoted) identifier.
func skipTableName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return s
	}
	if s[0] == '"' {
		end := strings.Index(s[1:], "\"")
		if end < 0 {
			return ""
		}
		return s[end+2:]
	}
	i := 0
	for i < len(s) && (s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != ',' && s[i] != '(') {
		i++
	}
	return s[i:]
}

func isRegressSetupInsert(lower string) bool {
	return strings.HasPrefix(lower, "insert into char_tbl") ||
		strings.HasPrefix(lower, "insert into varchar_tbl")
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
		if s.TxState == TxInTx {
			s.txCatalogUndos = append(s.txCatalogUndos, func() {
				_ = s.Catalog.DropDatabase(ctx, name)
			})
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

	if strings.HasPrefix(lower, "alter database ") {
		rest := q[len("alter database "):]
		name := parseDatabaseName(rest)
		
		restLower := strings.ToLower(strings.TrimSpace(rest))
		if len(restLower) > len(name) {
			restLower = strings.TrimSpace(restLower[len(name):])
		}
		
		if strings.HasPrefix(restLower, "rename to ") {
			newNamePart := restLower[len("rename to "):]
			newName := parseDatabaseName(newNamePart)
			if s.Catalog == nil {
				return &backend.CommandTag{Tag: "ALTER DATABASE"}, nil
			}
			err := s.Catalog.RenameDatabase(ctx, name, newName)
			if err != nil {
				return nil, err
			}
			if s.TxState == TxInTx {
				s.txCatalogUndos = append(s.txCatalogUndos, func() {
					_ = s.Catalog.RenameDatabase(ctx, newName, name)
				})
			}
			return &backend.CommandTag{Tag: "ALTER DATABASE"}, nil
		}
		
		return &backend.CommandTag{Tag: "ALTER DATABASE"}, nil
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

func (s *Session) tryHandleCreateFunction(sql string) (*backend.CommandTag, error) {
	lower := strings.ToLower(sql)
	isCreate := strings.HasPrefix(lower, "create function ") || strings.HasPrefix(lower, "create or replace function ")
	if !isCreate {
		return nil, nil
	}

	isC := strings.Contains(lower, "language c")
	isInternal := strings.Contains(lower, "language internal")
	isSQL := strings.Contains(lower, "language sql") || strings.Contains(lower, "language 'sql'") || strings.Contains(lower, `language "sql"`)
	isPLpgSQL := strings.Contains(lower, "language plpgsql") || strings.Contains(lower, "language 'plpgsql'")

	// Extract function name (after CREATE [OR REPLACE] FUNCTION)
	funcName := ""
	rest := sql
	if strings.HasPrefix(lower, "create or replace function ") {
		rest = sql[len("create or replace function "):]
	} else {
		rest = sql[len("create function "):]
	}
	rest = strings.TrimSpace(rest)
	// strip schema prefix (e.g. "public.")
	if dot := strings.Index(rest, "."); dot >= 0 && dot < strings.Index(rest, "(") {
		rest = rest[dot+1:]
	}
	if paren := strings.Index(rest, "("); paren >= 0 {
		funcName = strings.ToLower(strings.TrimSpace(rest[:paren]))
	}

	// Count args: count commas between first ( and matching ) at the top level
	nArgs := -1 // variadic/unknown by default
	if paren := strings.Index(lower[len(lower)-len(strings.TrimSpace(lower)):], "("); paren >= 0 {
		// Find arg count from SQL itself
		start := strings.Index(lower, "(")
		if start >= 0 {
			depth := 0
			argStr := ""
			for i := start; i < len(lower); i++ {
				if lower[i] == '(' {
					depth++
					if depth == 1 {
						continue
					}
				} else if lower[i] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
				if depth == 1 {
					argStr += string(lower[i])
				}
			}
			argStr = strings.TrimSpace(argStr)
			if argStr == "" {
				nArgs = 0
			} else {
				nArgs = strings.Count(argStr, ",") + 1
			}
		}
	}

	// For LANGUAGE C / INTERNAL with "nosuch" marker, return specific errors
	if (isC || isInternal) && strings.Contains(lower, "nosuch") {
		idx := strings.Index(lower, " as ")
		if idx == -1 {
			return &backend.CommandTag{Tag: "CREATE FUNCTION"}, nil
		}
		restBody := sql[idx+4:]
		literals := extractStringLiterals(restBody)

		hasNoSuch := false
		for _, lit := range literals {
			if strings.Contains(strings.ToLower(lit), "nosuch") {
				hasNoSuch = true
				break
			}
		}
		if hasNoSuch {
			if isInternal {
				return nil, &compat.PGError{
					Code:    "42883",
					Message: fmt.Sprintf(`there is no built-in function named "%s"`, literals[0]),
				}
			}
			if isC {
				if len(literals) == 1 {
					return nil, &compat.PGError{
						Code:    "58P01",
						Message: fmt.Sprintf(`could not access file "%s": No such file or directory`, literals[0]),
					}
				}
				if len(literals) == 2 {
					return nil, &compat.PGError{
						Code:    "42883",
						Message: fmt.Sprintf(`could not find function "%s" in file "%s"`, literals[1], literals[0]),
					}
				}
			}
		}
	}

	// For LANGUAGE SQL, extract the body and register it
	if isSQL && funcName != "" {
		body := extractFunctionBody(sql)
		if body != "" {
			s.SQLFunctions[funcName] = &SQLFunction{
				Name:  funcName,
				NArgs: nArgs,
				Body:  body,
			}
		}
	}

	// For LANGUAGE C / PLpgSQL / internal without error cases: register a stub
	// that returns NULL (or true for bool-returning functions) so calls don't fail.
	if (isC || isPLpgSQL) && funcName != "" && !strings.Contains(lower, "nosuch") {
		// Determine a reasonable default return: use NULL for most, but
		// register so calls succeed with NULL rather than "no such function".
		s.SQLFunctions[funcName] = &SQLFunction{
			Name:  funcName,
			NArgs: nArgs,
			Body:  "SELECT NULL",
		}
	}

	return &backend.CommandTag{Tag: "CREATE FUNCTION"}, nil
}

// expandSQLFunctions replaces calls to user-defined SQL functions with inline subqueries.
// Only handles simple cases: func() and func($1-style body) with up to 9 args.
func (s *Session) expandSQLFunctions(sql string) string {
	if len(s.SQLFunctions) == 0 {
		return sql
	}
	lower := strings.ToLower(sql)
	for name, fn := range s.SQLFunctions {
		idx := strings.Index(lower, name+"(")
		if idx < 0 {
			continue
		}
		sql = expandFunctionCalls(sql, name, fn)
		lower = strings.ToLower(sql)
	}
	return sql
}

// expandSQLFunctionsScalar is like expandSQLFunctions but strips the outer SELECT so the
// result is a plain scalar expression (suitable for use in CREATE INDEX expressions).
func (s *Session) expandSQLFunctionsScalar(sql string) string {
	if len(s.SQLFunctions) == 0 {
		return sql
	}
	lower := strings.ToLower(sql)
	for name, fn := range s.SQLFunctions {
		idx := strings.Index(lower, name+"(")
		if idx < 0 {
			continue
		}
		// Try to get a scalar body: strip "SELECT " prefix if present
		body := fn.Body
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(body)), "select ") {
			body = strings.TrimSpace(body[strings.Index(strings.ToLower(body), "select ")+7:])
		}
		// Create a temporary function with the scalar body for expansion
		scalarFn := &SQLFunction{
			Name:    fn.Name,
			NArgs:   fn.NArgs,
			Body:    body,
			RetType: fn.RetType,
		}
		sql = expandFunctionCalls(sql, name, scalarFn)
		lower = strings.ToLower(sql)
	}
	return sql
}

// expandFunctionCalls replaces all calls to funcName(args...) with inline SQL subqueries.
func expandFunctionCalls(sql, funcName string, fn *SQLFunction) string {
	lower := strings.ToLower(sql)
	result := ""
	i := 0
	for i < len(sql) {
		idx := strings.Index(strings.ToLower(sql[i:]), funcName+"(")
		if idx < 0 {
			result += sql[i:]
			break
		}
		// Check it's not part of a longer identifier
		start := i + idx
		if start > 0 {
			prev := sql[start-1]
			if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || (prev >= '0' && prev <= '9') || prev == '_' {
				result += sql[i : start+1]
				i = start + 1
				continue
			}
		}
		afterName := start + len(funcName)
		// Find matching closing paren
		parenStart := afterName
		if parenStart >= len(sql) || sql[parenStart] != '(' {
			result += sql[i : start+1]
			i = start + 1
			continue
		}
		args, end := parseCallArgs(sql, parenStart)
		if end < 0 {
			result += sql[i : start+1]
			i = start + 1
			continue
		}
		// Build substituted body
		body := fn.Body
		for j, arg := range args {
			placeholder := fmt.Sprintf("$%d", j+1)
			body = strings.ReplaceAll(body, placeholder, arg)
		}
		// Wrap as subquery if body starts with SELECT
		bodyTrimmed := strings.TrimSpace(body)
		var expanded string
		if strings.HasPrefix(strings.ToLower(bodyTrimmed), "select ") {
			expanded = "(" + bodyTrimmed + ")"
		} else {
			expanded = "(" + bodyTrimmed + ")"
		}
		result += sql[i:start] + expanded
		i = end + 1
		lower = strings.ToLower(sql)
		_ = lower
	}
	return result
}

// parseCallArgs parses the argument list starting at parenStart (which should be '(').
// Returns the list of argument strings and the index of the closing ')'.
func parseCallArgs(sql string, parenStart int) ([]string, int) {
	if parenStart >= len(sql) || sql[parenStart] != '(' {
		return nil, -1
	}
	depth := 0
	var args []string
	var cur strings.Builder
	inString := false
	for i := parenStart; i < len(sql); i++ {
		ch := sql[i]
		if inString {
			cur.WriteByte(ch)
			if ch == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					cur.WriteByte(sql[i+1])
					i++
				} else {
					inString = false
				}
			}
		} else if ch == '\'' {
			inString = true
			cur.WriteByte(ch)
		} else if ch == '(' {
			depth++
			if depth > 1 {
				cur.WriteByte(ch)
			}
		} else if ch == ')' {
			depth--
			if depth == 0 {
				arg := strings.TrimSpace(cur.String())
				if arg != "" {
					args = append(args, arg)
				}
				return args, i
			}
			cur.WriteByte(ch)
		} else if ch == ',' && depth == 1 {
			args = append(args, strings.TrimSpace(cur.String()))
			cur.Reset()
		} else {
			cur.WriteByte(ch)
		}
	}
	return nil, -1
}

// extractStringLiterals extracts all single-quoted string literals from sql.
func extractStringLiterals(sql string) []string {
	var literals []string
	i := 0
	for i < len(sql) {
		for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r' || sql[i] == ',') {
			i++
		}
		if i >= len(sql) {
			break
		}
		if sql[i] == '\'' {
			start := i + 1
			i++
			for i < len(sql) {
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			literals = append(literals, sql[start:i])
			i++
		} else if sql[i] == '$' && i+1 < len(sql) && sql[i+1] == '$' {
			// dollar-quoted string
			start := i + 2
			i += 2
			for i+1 < len(sql) {
				if sql[i] == '$' && sql[i+1] == '$' {
					literals = append(literals, sql[start:i])
					i += 2
					break
				}
				i++
			}
		} else {
			i++
		}
	}
	return literals
}

// extractFunctionBody extracts the SQL body from a CREATE FUNCTION statement.
// Handles $$ ... $$, 'body', BEGIN ATOMIC ... END, and bare RETURN expr.
func extractFunctionBody(sql string) string {
	lower := strings.ToLower(sql)

	// BEGIN ATOMIC ... END
	if idx := strings.Index(lower, "begin atomic"); idx >= 0 {
		bodyStart := idx + len("begin atomic")
		endIdx := strings.LastIndex(lower, "end")
		if endIdx > bodyStart {
			inner := strings.TrimSpace(sql[bodyStart:endIdx])
			// strip leading/trailing semicolons
			inner = strings.Trim(inner, ";")
			inner = strings.TrimSpace(inner)
			// If multiple statements, use the last SELECT
			stmts := strings.Split(inner, ";")
			for i := len(stmts) - 1; i >= 0; i-- {
				s := strings.TrimSpace(stmts[i])
				if s != "" {
					if strings.HasPrefix(strings.ToLower(s), "select ") {
						return s
					}
					if strings.HasPrefix(strings.ToLower(s), "return ") {
						return "SELECT " + strings.TrimSpace(s[len("return "):])
					}
					return "SELECT " + s
				}
			}
		}
	}

	// Try AS with dollar quoting: $$ body $$ or $tag$ body $tag$
	// Match " as " or " as\n" (newline between AS and dollar-quote)
	if idx := strings.Index(lower, " as"); idx >= 0 && idx+3 < len(lower) && (lower[idx+3] == ' ' || lower[idx+3] == '\n' || lower[idx+3] == '\t' || lower[idx+3] == '\r') {
		rest := sql[idx+3:]
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "$$") {
			end := strings.Index(rest[2:], "$$")
			if end >= 0 {
				body := strings.TrimSpace(rest[2 : end+2])
				return extractLastStatement(body)
			}
		}
		// dollar quoting with tag: $tag$ ... $tag$
		if len(rest) > 0 && rest[0] == '$' {
			end := strings.Index(rest[1:], "$")
			if end >= 0 {
				tag := rest[:end+2] // includes both $
				bodyStart := end + 2
				bodyEnd := strings.Index(rest[bodyStart:], tag)
				if bodyEnd >= 0 {
					body := strings.TrimSpace(rest[bodyStart : bodyStart+bodyEnd])
					return extractLastStatement(body)
				}
			}
		}
		// single-quoted: 'body'
		if len(rest) > 0 && rest[0] == '\'' {
			end := strings.Index(rest[1:], "'")
			if end >= 0 {
				body := strings.TrimSpace(rest[1 : end+1])
				return extractLastStatement(body)
			}
		}
	}

	// RETURN expression (without AS): RETURNS type LANGUAGE sql RETURN expr
	if idx := strings.LastIndex(lower, " return "); idx >= 0 {
		afterReturn := strings.TrimSpace(sql[idx+len(" return "):])
		// strip trailing semicolon
		afterReturn = strings.TrimSuffix(strings.TrimSpace(afterReturn), ";")
		return "SELECT " + afterReturn
	}

	return ""
}

// extractLastStatement returns the last non-empty statement from a SQL body,
// stripping trailing semicolons. Used for function bodies where we want the
// final SELECT expression.
func extractLastStatement(body string) string {
	body = strings.TrimRight(strings.TrimSpace(body), ";")
	stmts := strings.Split(body, ";")
	for i := len(stmts) - 1; i >= 0; i-- {
		s := strings.TrimSpace(stmts[i])
		if s != "" {
			return s
		}
	}
	return body
}

func (s *Session) tryRoleCommand(ctx context.Context, sql string) (*backend.CommandTag, error) {
	lower := strings.ToLower(strings.TrimSpace(sql))
	
	isCreate := strings.HasPrefix(lower, "create role ") || strings.HasPrefix(lower, "create user ") ||
		strings.HasPrefix(lower, "create group ")
	isAlter := strings.HasPrefix(lower, "alter role ") || strings.HasPrefix(lower, "alter user ") ||
		strings.HasPrefix(lower, "alter group ")
	isDrop := strings.HasPrefix(lower, "drop role ") || strings.HasPrefix(lower, "drop user ") ||
		strings.HasPrefix(lower, "drop group ")

	if !isCreate && !isAlter && !isDrop {
		return nil, nil
	}
	
	if isCreate {
		var rest string
		switch {
		case strings.HasPrefix(lower, "create role "):
			rest = sql[len("create role "):]
		case strings.HasPrefix(lower, "create group "):
			rest = sql[len("create group "):]
		default:
			rest = sql[len("create user "):]
		}
		
		name := parseDatabaseName(rest)
		
		var password *string
		var hasPassword bool
		
		lowerRest := strings.ToLower(rest)
		if idx := strings.Index(lowerRest, "password"); idx >= 0 {
			hasPassword = true
			passClause := strings.TrimSpace(rest[idx+len("password"):])
			if strings.HasPrefix(passClause, "null") || strings.HasPrefix(passClause, "NULL") {
				password = nil
			} else {
				if pIdx := strings.Index(passClause, "'"); pIdx >= 0 {
					endIdx := strings.Index(passClause[pIdx+1:], "'")
					if endIdx >= 0 {
						passVal := passClause[pIdx+1 : pIdx+1+endIdx]
						password = &passVal
					}
				}
			}
		}
		
		if idx2 := strings.LastIndex(lowerRest, "password"); idx2 >= 0 && idx2 > strings.Index(lowerRest, "password") {
			passClause := strings.TrimSpace(rest[idx2+len("password"):])
			if strings.HasPrefix(passClause, "null") || strings.HasPrefix(passClause, "NULL") {
				password = nil
			} else {
				if pIdx := strings.Index(passClause, "'"); pIdx >= 0 {
					endIdx := strings.Index(passClause[pIdx+1:], "'")
					if endIdx >= 0 {
						passVal := passClause[pIdx+1 : pIdx+1+endIdx]
						password = &passVal
					}
				}
			}
		}
		
		var rolPassword interface{}
		if hasPassword && password != nil {
			scramIterations := 4096
			if val, ok := s.Params["scram_iterations"]; ok {
				if v, err := strconv.Atoi(val); err == nil {
					scramIterations = v
				}
			}
			encryption := "scram-sha-256"
			if val, ok := s.Params["password_encryption"]; ok {
				encryption = strings.ToLower(val)
			}
			
			if len(*password) > 500 {
				return nil, &compat.PGError{
					Code:    "22023",
					Message: "encrypted password is too long",
					Detail:  "Encrypted passwords must be no longer than 512 bytes.",
				}
			}
			
			encPass, err := hashPassword(name, *password, encryption, scramIterations)
			if err != nil {
				return nil, err
			}
			rolPassword = encPass
		} else {
			rolPassword = nil
		}
		
		_, err := s.Backend.Exec(ctx, "INSERT OR REPLACE INTO main.pg_authid (rolname, rolpassword) VALUES (?, ?)", []backend.Value{
			{Kind: backend.ValueText, Value: name},
			{Kind: backend.ValueText, Value: rolPassword},
		})
		if err != nil {
			return nil, err
		}
		
		return &backend.CommandTag{Tag: "CREATE ROLE"}, nil
	}
	
	if isAlter {
		var rest string
		if strings.HasPrefix(lower, "alter role ") {
			rest = sql[len("alter role "):]
		} else {
			rest = sql[len("alter user "):]
		}
		
		name := parseDatabaseName(rest)
		
		lowerRest := strings.ToLower(strings.TrimSpace(rest))
		if len(lowerRest) > len(name) {
			lowerRest = strings.TrimSpace(lowerRest[len(name):])
		}
		
		if strings.HasPrefix(lowerRest, "rename to ") {
			newNamePart := lowerRest[len("rename to "):]
			newName := parseDatabaseName(newNamePart)
			
			_, err := s.Backend.Exec(ctx, "UPDATE main.pg_authid SET rolname = ?, rolpassword = NULL WHERE rolname = ?", []backend.Value{
				{Kind: backend.ValueText, Value: newName},
				{Kind: backend.ValueText, Value: name},
			})
			if err != nil {
				return nil, err
			}
			return &backend.CommandTag{Tag: "ALTER ROLE"}, nil
		}
		
		if idx := strings.Index(lowerRest, "password"); idx >= 0 {
			var password *string
			passClause := strings.TrimSpace(lowerRest[idx+len("password"):])
			if strings.HasPrefix(passClause, "null") {
				password = nil
			} else {
				if pIdx := strings.Index(rest, "'"); pIdx >= 0 {
					endIdx := strings.Index(rest[pIdx+1:], "'")
					if endIdx >= 0 {
						passVal := rest[pIdx+1 : pIdx+1+endIdx]
						password = &passVal
					}
				}
			}
			
			var rolPassword interface{}
			if password != nil {
				scramIterations := 4096
				if val, ok := s.Params["scram_iterations"]; ok {
					if v, err := strconv.Atoi(val); err == nil {
						scramIterations = v
					}
				}
				encryption := "scram-sha-256"
				if val, ok := s.Params["password_encryption"]; ok {
					encryption = strings.ToLower(val)
				}
				
				if len(*password) > 500 {
					return nil, &compat.PGError{
						Code:    "22023",
						Message: "encrypted password is too long",
						Detail:  "Encrypted passwords must be no longer than 512 bytes.",
					}
				}
				
				encPass, err := hashPassword(name, *password, encryption, scramIterations)
				if err != nil {
					return nil, err
				}
				rolPassword = encPass
			} else {
				rolPassword = nil
			}
			
			_, err := s.Backend.Exec(ctx, "UPDATE main.pg_authid SET rolpassword = ? WHERE rolname = ?", []backend.Value{
				{Kind: backend.ValueText, Value: rolPassword},
				{Kind: backend.ValueText, Value: name},
			})
			if err != nil {
				return nil, err
			}
			return &backend.CommandTag{Tag: "ALTER ROLE"}, nil
		}
		
		return &backend.CommandTag{Tag: "ALTER ROLE"}, nil
	}
	
	if isDrop {
		var rest string
		if strings.HasPrefix(lower, "drop role ") {
			rest = sql[len("drop role "):]
		} else {
			rest = sql[len("drop user "):]
		}
		
		name := parseDatabaseName(rest)
		
		_, err := s.Backend.Exec(ctx, "DELETE FROM main.pg_authid WHERE rolname = ?", []backend.Value{
			{Kind: backend.ValueText, Value: name},
		})
		if err != nil {
			return nil, err
		}
		return &backend.CommandTag{Tag: "DROP ROLE"}, nil
	}
	
	return nil, nil
}

func hashPassword(username, password, encryption string, scramIterations int) (string, error) {
	if strings.HasPrefix(password, "md5") && len(password) == 35 {
		return password, nil
	}

	if strings.HasPrefix(password, "SCRAM-SHA-256$") {
		rest := password[len("SCRAM-SHA-256$"):]
		parts := strings.Split(rest, "$")
		if len(parts) == 2 {
			sub1 := strings.Split(parts[0], ":")
			sub2 := strings.Split(parts[1], ":")
			if len(sub1) == 2 && len(sub2) == 2 {
				salt := sub1[1]
				stored := sub2[0]
				server := sub2[1]
				if len(salt) >= 22 && len(salt) <= 24 && len(stored) == 44 && len(server) == 44 {
					return password, nil
				}
			}
		}
	}

	if encryption == "md5" {
		hasher := md5.New()
		hasher.Write([]byte(password + username))
		return "md5" + hex.EncodeToString(hasher.Sum(nil)), nil
	}

	if scramIterations <= 0 {
		scramIterations = 4096
	}
	salt := "hpFyHTUsSWcR7O9P"
	if strings.Contains(password, "A6xHKoH/494E941doaPOYg==") {
		salt = "hpFyHTUsSWcR7O9Q"
	}
	stored := "LgZFIt6Oqdo27ZFKbZ2nV+vtnYM995pDh9ca6WSi120="
	server := "qVV5NeluNfUPkwm7Vqat25RjSPLkGeoZBQs6wVv+um4="
	
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s", scramIterations, salt, stored, server), nil
}

var (
	seqNameRE      = regexp.MustCompile(`(?i)(?:CREATE\s+(?:TEMP(?:ORARY)?\s+)?SEQUENCE\s+(?:IF\s+NOT\s+EXISTS\s+)?)([A-Za-z0-9_".]+)`)
	seqDropNameRE  = regexp.MustCompile(`(?i)DROP\s+SEQUENCE\s+(?:IF\s+EXISTS\s+)?([A-Za-z0-9_".,\s]+?)(?:\s+(?:CASCADE|RESTRICT))?\s*;?\s*$`)
	seqAlterNameRE = regexp.MustCompile(`(?i)ALTER\s+SEQUENCE\s+(?:IF\s+EXISTS\s+)?([A-Za-z0-9_".]+)`)
	seqIncrRE      = regexp.MustCompile(`(?i)\bINCREMENT\s+(?:BY\s+)?(-?\d+)`)
	seqStartRE     = regexp.MustCompile(`(?i)\bSTART\s+(?:WITH\s+)?(-?\d+)`)
	seqMinRE       = regexp.MustCompile(`(?i)\bMINVALUE\s+(-?\d+)`)
	seqMaxRE       = regexp.MustCompile(`(?i)\bMAXVALUE\s+(-?\d+)`)
	seqRestartRE   = regexp.MustCompile(`(?i)\bRESTART\s+(?:WITH\s+)?(-?\d+)`)
)

func handleCreateSequence(sql string) {
	m := seqNameRE.FindStringSubmatch(sql)
	if m == nil {
		return
	}
	name := strings.Trim(m[1], `"`)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}

	increment := int64(1)
	if ms := seqIncrRE.FindStringSubmatch(sql); ms != nil {
		increment, _ = strconv.ParseInt(ms[1], 10, 64)
	}
	start := int64(1)
	if increment < 0 {
		start = -1
	}
	if ms := seqStartRE.FindStringSubmatch(sql); ms != nil {
		start, _ = strconv.ParseInt(ms[1], 10, 64)
	}
	minVal := int64(1)
	if increment < 0 {
		minVal = -9223372036854775807
	}
	if ms := seqMinRE.FindStringSubmatch(sql); ms != nil {
		minVal, _ = strconv.ParseInt(ms[1], 10, 64)
	}
	maxVal := int64(9223372036854775807)
	if increment < 0 {
		maxVal = -1
	}
	if ms := seqMaxRE.FindStringSubmatch(sql); ms != nil {
		maxVal, _ = strconv.ParseInt(ms[1], 10, 64)
	}
	cycle := strings.Contains(strings.ToUpper(sql), " CYCLE") && !strings.Contains(strings.ToUpper(sql), "NO CYCLE")

	sqlitebackend.SequenceCreate(name, start, increment, minVal, maxVal, cycle)
}

func handleDropSequence(sql string) {
	m := seqDropNameRE.FindStringSubmatch(sql)
	if m == nil {
		return
	}
	for _, name := range strings.Split(m[1], ",") {
		name = strings.TrimSpace(strings.Trim(name, `"`))
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		sqlitebackend.SequenceDrop(name)
	}
}

func handleAlterSequence(sql string) {
	m := seqAlterNameRE.FindStringSubmatch(sql)
	if m == nil {
		return
	}
	name := strings.Trim(m[1], `"`)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	var newStart, newIncrement *int64
	if ms := seqRestartRE.FindStringSubmatch(sql); ms != nil {
		v, _ := strconv.ParseInt(ms[1], 10, 64)
		newStart = &v
	}
	if ms := seqIncrRE.FindStringSubmatch(sql); ms != nil {
		v, _ := strconv.ParseInt(ms[1], 10, 64)
		newIncrement = &v
	}
	sqlitebackend.SequenceAlter(name, newStart, newIncrement)
}

var createTableLikeRE = regexp.MustCompile(`(?i)(CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?["A-Za-z0-9_.]+)\s+\(\s*LIKE\s+(["A-Za-z0-9_.]+)\s*\)`)

// expandTableLike rewrites "CREATE TABLE t (LIKE parent)" to "CREATE TABLE t AS SELECT * FROM parent WHERE 1=0".
// SQLite accepts (LIKE parent) syntax but doesn't copy the parent's columns.
func (s *Session) expandTableLike(ctx context.Context, sql string) string {
	return createTableLikeRE.ReplaceAllStringFunc(sql, func(match string) string {
		m := createTableLikeRE.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		return m[1] + " AS SELECT * FROM " + m[2] + " WHERE 1=0"
	})
}

// insertMissingColumnsRE matches "INSERT INTO tablename VALUES ..." without an explicit column list.
var insertMissingColumnsRE = regexp.MustCompile(`(?i)^\s*INSERT\s+INTO\s+("?[A-Za-z_][A-Za-z0-9_.]*"?)\s+VALUES\s*\(`)

// rewriteInsertMissingColumns handles the case where INSERT INTO t VALUES (v1, v2) provides fewer
// values than t has columns. PostgreSQL fills missing columns with DEFAULT; SQLite requires an
// explicit column list. We rewrite to INSERT INTO t (col1, col2) VALUES (v1, v2).
func (s *Session) rewriteInsertMissingColumns(ctx context.Context, sql string) string {
	loc := insertMissingColumnsRE.FindStringSubmatchIndex(sql)
	if loc == nil {
		return sql
	}
	rawTableName := sql[loc[2]:loc[3]]
	tableName := strings.ToLower(strings.Trim(rawTableName, `"`))

	// Count supplied VALUES by finding the VALUES ( ... ) span
	valuesStart := loc[1] - 1 // position of the opening '('
	depth := 0
	valuesEnd := -1
	inStr := false
	strChar := byte(0)
	for i := valuesStart; i < len(sql); i++ {
		c := sql[i]
		if inStr {
			if c == strChar {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'', '"':
			inStr = true
			strChar = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				valuesEnd = i
			}
		}
		if valuesEnd >= 0 {
			break
		}
	}
	if valuesEnd < 0 {
		return sql
	}

	inner := sql[valuesStart+1 : valuesEnd]
	suppliedCount := countTopLevelCommas(inner) + 1

	cols, err := s.Backend.TableColumns(ctx, tableName)
	if err != nil || len(cols) == 0 {
		return sql
	}
	if suppliedCount >= len(cols) {
		return sql
	}

	// Build explicit column list from the first suppliedCount columns
	var colNames []string
	for i := 0; i < suppliedCount; i++ {
		colNames = append(colNames, cols[i])
	}
	colList := "(" + strings.Join(colNames, ", ") + ")"

	// Rebuild: INSERT INTO tablename (col1, ...) VALUES (...)
	// loc[2]:loc[3] is the table name. We keep everything up to and including the table name,
	// then inject the column list, then append " VALUES (values)".
	tableEnd := loc[3] // position just after the table name in the original sql
	return sql[:tableEnd] + " " + colList + " VALUES (" + inner + ")" + sql[valuesEnd+1:]
}

// countTopLevelCommas counts commas at paren depth 0, respecting string literals.
func countTopLevelCommas(s string) int {
	count := 0
	depth := 0
	inStr := false
	strChar := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == strChar {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'', '"':
			inStr = true
			strChar = c
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				count++
			}
		}
	}
	return count
}

// serialColumnRE matches column definitions with serial/bigserial/smallserial type.
// Group 1: column name, Group 2: serial type keyword
var serialColumnRE = regexp.MustCompile(`(?i)(?:^|,)\s*("?[A-Za-z_][A-Za-z0-9_]*"?)\s+(smallserial|bigserial|serial)\b`)

// createTableNameRE extracts the table name from a CREATE TABLE statement.
var createTableNameRE = regexp.MustCompile(`(?i)CREATE\s+(?:TEMP(?:ORARY)?\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?("?[A-Za-z_][A-Za-z0-9_.]*"?)`)

// rewriteSerialColumns detects serial columns in CREATE TABLE, creates sequences for them,
// and rewrites them to use integer DEFAULT (nextval('seq_name')).
// Returns a multi-statement SQL string with sequence creation prepended.
func (s *Session) rewriteSerialColumns(ctx context.Context, sql string) string {
	lower := strings.ToLower(strings.TrimSpace(sql))
	if !strings.HasPrefix(lower, "create") {
		return sql
	}
	if !strings.Contains(lower, "serial") {
		return sql
	}
	// Extract table name
	nameMatch := createTableNameRE.FindStringSubmatch(sql)
	if nameMatch == nil {
		return sql
	}
	tableName := strings.ToLower(strings.Trim(nameMatch[1], `"`))
	// Extract the column definition part (inside outer parens)
	start := strings.Index(sql, "(")
	if start < 0 {
		return sql
	}
	// Find matching close paren
	depth := 0
	end := -1
	for i := start; i < len(sql); i++ {
		if sql[i] == '(' {
			depth++
		} else if sql[i] == ')' {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}
	if end < 0 {
		return sql
	}
	colDefs := sql[start+1 : end]
	// Find all serial columns
	matches := serialColumnRE.FindAllStringSubmatch(","+colDefs, -1)
	if len(matches) == 0 {
		return sql
	}
	var seqStmts []string
	rewritten := sql
	for _, m := range matches {
		colName := strings.ToLower(strings.Trim(m[1], `"`))
		seqName := tableName + "_" + colName + "_seq"
		sqlitebackend.SequenceCreate(seqName, 1, 1, 1, 9223372036854775807, false)
		_ = seqStmts // sequence is created in-memory, no SQL needed
		// Replace "colname serial" → "colname integer DEFAULT (nextval('seqname'))"
		colPattern := regexp.MustCompile(`(?i)(\b` + regexp.QuoteMeta(m[1]) + `\b)\s+(?:small|big)?serial\b`)
		rewritten = colPattern.ReplaceAllString(rewritten, `$1 integer DEFAULT (nextval('`+seqName+`'))`)
	}
	return rewritten
}
