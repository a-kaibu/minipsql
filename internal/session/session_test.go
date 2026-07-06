package session_test

import (
	"context"
	"testing"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/backend/sqlite"
	"github.com/yashikota/minipsql/internal/session"
)

func TestSessionBasicDDLDML(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(1, 1, "default", "mini", b, nil)
	execOK(t, ctx, s, "create table users(id integer primary key, name text)")
	execOK(t, ctx, s, "insert into users values (1, 'alice')")

	result := queryOK(t, ctx, s, "select * from users")
	if got := result.Rows[0][1].Value; got != "alice" {
		t.Fatalf("name before update = %v, want alice", got)
	}

	execOK(t, ctx, s, "update users set name = 'bob' where id = 1")
	result = queryOK(t, ctx, s, "select * from users")
	if got := result.Rows[0][1].Value; got != "bob" {
		t.Fatalf("name after update = %v, want bob", got)
	}

	execOK(t, ctx, s, "delete from users where id = 1")
	result = queryOK(t, ctx, s, "select * from users")
	if len(result.Rows) != 0 {
		t.Fatalf("rows after delete = %d, want 0", len(result.Rows))
	}
}

func TestSessionBuiltins(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(1, 1, "default", "mini", b, nil)
	cases := map[string]string{
		"select version()":                 "PostgreSQL 14.0 compatible minipsql",
		"show server_version":              "14.0",
		"show server_version_num":          "140000",
		"show client_encoding":             "UTF8",
		"show standard_conforming_strings": "on",
		"show search_path":                 "\"$user\", public",
		"select current_database()":        "default",
		"select current_schema()":          "public",
		"select current_user":              "mini",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			result := queryOK(t, ctx, s, sql)
			if got := result.Rows[0][0].Value; got != want {
				t.Fatalf("%s = %v, want %v", sql, got, want)
			}
		})
	}
}

func TestSessionTransactions(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(1, 1, "default", "mini", b, nil)
	execOK(t, ctx, s, "create table users(id integer primary key, name text)")

	execOK(t, ctx, s, "begin")
	if s.TxState != session.TxInTx {
		t.Fatalf("tx state after begin = %q, want %q", s.TxState, session.TxInTx)
	}
	execOK(t, ctx, s, "insert into users values (1, 'alice')")
	execOK(t, ctx, s, "rollback")
	if s.TxState != session.TxIdle {
		t.Fatalf("tx state after rollback = %q, want %q", s.TxState, session.TxIdle)
	}
	result := queryOK(t, ctx, s, "select * from users")
	if len(result.Rows) != 0 {
		t.Fatalf("rows after rollback = %d, want 0", len(result.Rows))
	}

	execOK(t, ctx, s, "start transaction")
	execOK(t, ctx, s, "insert into users values (1, 'alice')")
	execOK(t, ctx, s, "commit")
	result = queryOK(t, ctx, s, "select name from users where id = 1")
	if got := result.Rows[0][0].Value; got != "alice" {
		t.Fatalf("name after commit = %v, want alice", got)
	}
}

func TestInformationSchema(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(1, 1, "default", "mini", b, nil)
	execOK(t, ctx, s, "create table users(id integer primary key, name text)")

	tables := queryOK(t, ctx, s, "select * from information_schema.tables")
	if len(tables.Rows) != 1 {
		t.Fatalf("information_schema.tables rows = %d, want 1", len(tables.Rows))
	}
	if got := tables.Rows[0][2].Value; got != "users" {
		t.Fatalf("table_name = %v, want users", got)
	}

	columns := queryOK(t, ctx, s, "select * from information_schema.columns")
	if len(columns.Rows) != 2 {
		t.Fatalf("information_schema.columns rows = %d, want 2", len(columns.Rows))
	}
	if got := columns.Rows[0][3].Value; got != "id" {
		t.Fatalf("first column_name = %v, want id", got)
	}
	if got := columns.Rows[1][3].Value; got != "name" {
		t.Fatalf("second column_name = %v, want name", got)
	}
}

func TestPGCatalog(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(1, 1, "default", "mini", b, nil)
	execOK(t, ctx, s, "create table users(id integer primary key, name text)")

	namespace := queryOK(t, ctx, s, "select * from pg_catalog.pg_namespace")
	if got := namespace.Rows[0][1].Value; got != "public" {
		t.Fatalf("nspname = %v, want public", got)
	}
	types := queryOK(t, ctx, s, "select * from pg_catalog.pg_type")
	if len(types.Rows) == 0 {
		t.Fatal("pg_type returned no rows")
	}
	classes := queryOK(t, ctx, s, "select * from pg_catalog.pg_class")
	if got := classes.Rows[0][1].Value; got != "users" {
		t.Fatalf("relname = %v, want users", got)
	}
	attrs := queryOK(t, ctx, s, "select * from pg_catalog.pg_attribute")
	if len(attrs.Rows) != 2 {
		t.Fatalf("pg_attribute rows = %d, want 2", len(attrs.Rows))
	}
	tables := queryOK(t, ctx, s, "select * from pg_catalog.pg_tables")
	if got := tables.Rows[0][1].Value; got != "users" {
		t.Fatalf("tablename = %v, want users", got)
	}
}

func execOK(t *testing.T, ctx context.Context, s *session.Session, sql string) {
	t.Helper()
	result, tag, err := s.Execute(ctx, sql, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil || tag == nil {
		t.Fatalf("Execute(%q) returned result=%v tag=%v, want command tag", sql, result, tag)
	}
}

func queryOK(t *testing.T, ctx context.Context, s *session.Session, sql string, args ...backend.Value) *backend.Result {
	t.Helper()
	result, tag, err := s.Execute(ctx, sql, args)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || tag != nil {
		t.Fatalf("Execute(%q) returned result=%v tag=%v, want result", sql, result, tag)
	}
	return result
}
