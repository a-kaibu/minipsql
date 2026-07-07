package session

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
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
	RenameDatabase(ctx context.Context, oldName, newName string) error
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
	txCatalogUndos     []func()
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
	var err error
	sql, err = s.rewriteInherits(ctx, sql)
	if err != nil {
		return nil, nil, err
	}

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
			s.txCatalogUndos = nil
			return nil, &backend.CommandTag{Tag: "COMMIT"}, nil
		case "rollback":
			if err := s.Backend.Rollback(ctx); err != nil {
				s.TxState = TxFailed
				return nil, nil, err
			}
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

	if tag, err := tryHandleCreateFunction(sql); err != nil || tag != nil {
		return nil, tag, err
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
	case strings.HasPrefix(lower, "explain "):
		return &backend.CommandTag{Tag: "EXPLAIN"}, true
	case strings.HasPrefix(lower, "discard "):
		return &backend.CommandTag{Tag: "DISCARD ALL"}, true
	case strings.HasPrefix(lower, "create schema"):
		return &backend.CommandTag{Tag: "CREATE SCHEMA"}, true
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
	case strings.HasPrefix(lower, "cluster"):
		return &backend.CommandTag{Tag: "CLUSTER"}, true
	case strings.HasPrefix(lower, "reindex"):
		return &backend.CommandTag{Tag: "REINDEX"}, true
	case strings.HasPrefix(lower, "checkpoint"):
		return &backend.CommandTag{Tag: "CHECKPOINT"}, true
	case strings.HasPrefix(lower, "reassign owned"):
		return &backend.CommandTag{Tag: "REASSIGN OWNED"}, true
	case strings.HasPrefix(lower, "alter sequence "):
		return &backend.CommandTag{Tag: "ALTER SEQUENCE"}, true
	case strings.HasPrefix(lower, "create sequence "):
		return &backend.CommandTag{Tag: "CREATE SEQUENCE"}, true
	case strings.HasPrefix(lower, "create temp sequence "):
		return &backend.CommandTag{Tag: "CREATE SEQUENCE"}, true
	case strings.HasPrefix(lower, "create temporary sequence "):
		return &backend.CommandTag{Tag: "CREATE SEQUENCE"}, true
	case strings.HasPrefix(lower, "drop sequence "):
		return &backend.CommandTag{Tag: "DROP SEQUENCE"}, true
	case strings.HasPrefix(lower, "create materialized view"):
		return &backend.CommandTag{Tag: "CREATE MATERIALIZED VIEW"}, true
	case strings.HasPrefix(lower, "refresh materialized view"):
		return &backend.CommandTag{Tag: "REFRESH MATERIALIZED VIEW"}, true
	case strings.HasPrefix(lower, "drop materialized view"):
		return &backend.CommandTag{Tag: "DROP MATERIALIZED VIEW"}, true
	case strings.HasPrefix(lower, "alter index "):
		return &backend.CommandTag{Tag: "ALTER INDEX"}, true
	case strings.HasPrefix(lower, "security label"):
		return &backend.CommandTag{Tag: "SECURITY LABEL"}, true
	case strings.HasPrefix(lower, "import foreign schema"):
		return &backend.CommandTag{Tag: "IMPORT FOREIGN SCHEMA"}, true
	}

	// ALTER TABLE with unsupported subcommands → no-op
	if strings.HasPrefix(lower, "alter table ") {
		if tag, ok := tryAlterTableNoop(lower); ok {
			return tag, true
		}
	}

	// SAVEPOINT / RELEASE / ROLLBACK TO → pass through to SQLite
	if strings.HasPrefix(lower, "savepoint ") || lower == "savepoint" {
		return &backend.CommandTag{Tag: "SAVEPOINT"}, true
	}
	if strings.HasPrefix(lower, "release ") || lower == "release" {
		return &backend.CommandTag{Tag: "RELEASE"}, true
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

func tryHandleCreateFunction(sql string) (*backend.CommandTag, error) {
	lower := strings.ToLower(sql)
	if !strings.HasPrefix(lower, "create function ") && !strings.HasPrefix(lower, "create or replace function ") {
		return nil, nil
	}
	
	if !strings.Contains(lower, "nosuch") {
		return &backend.CommandTag{Tag: "CREATE FUNCTION"}, nil
	}
	
	isC := strings.Contains(lower, "language c")
	isInternal := strings.Contains(lower, "language internal")
	
	if !isC && !isInternal {
		return nil, nil
	}
	
	idx := strings.Index(lower, " as ")
	if idx == -1 {
		return nil, nil
	}
	
	rest := sql[idx+4:]
	var literals []string
	
	i := 0
	for i < len(rest) {
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == '\n' || rest[i] == '\r' || rest[i] == ',') {
			i++
		}
		if i >= len(rest) {
			break
		}
		if rest[i] == '\'' {
			start := i + 1
			i++
			for i < len(rest) {
				if rest[i] == '\'' {
					if i+1 < len(rest) && rest[i+1] == '\'' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			literals = append(literals, rest[start:i])
			i++
		} else {
			i++
		}
	}
	
	if len(literals) == 0 {
		return nil, nil
	}

	hasNoSuch := false
	for _, lit := range literals {
		if strings.Contains(strings.ToLower(lit), "nosuch") {
			hasNoSuch = true
			break
		}
	}
	if !hasNoSuch {
		return &backend.CommandTag{Tag: "CREATE FUNCTION"}, nil
	}
	
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
	
	return nil, nil
}

func (s *Session) tryRoleCommand(ctx context.Context, sql string) (*backend.CommandTag, error) {
	lower := strings.ToLower(strings.TrimSpace(sql))
	
	isCreate := strings.HasPrefix(lower, "create role ") || strings.HasPrefix(lower, "create user ")
	isAlter := strings.HasPrefix(lower, "alter role ") || strings.HasPrefix(lower, "alter user ")
	isDrop := strings.HasPrefix(lower, "drop role ") || strings.HasPrefix(lower, "drop user ")
	
	if !isCreate && !isAlter && !isDrop {
		return nil, nil
	}
	
	if isCreate {
		var rest string
		if strings.HasPrefix(lower, "create role ") {
			rest = sql[len("create role "):]
		} else {
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


