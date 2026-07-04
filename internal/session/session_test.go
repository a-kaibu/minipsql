package session_test

import (
	"context"
	"testing"

	"github.com/yashikota/minipostgresql/internal/backend"
	"github.com/yashikota/minipostgresql/internal/backend/sqlite"
	"github.com/yashikota/minipostgresql/internal/session"
)

func TestSessionBasicDDLDML(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close(ctx)

	s := session.New(1, 1, "default", "mini", b)
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
	defer b.Close(ctx)

	s := session.New(1, 1, "default", "mini", b)
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
