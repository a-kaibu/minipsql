package pgwire_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/yashikota/minipsql/internal/catalog"
	"github.com/yashikota/minipsql/internal/compat"
	"github.com/yashikota/minipsql/internal/pgwire"
)

func TestPGXSelectTypes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	addr, stop := startTestServer(t, ctx)
	defer stop()

	conn, err := pgx.Connect(ctx, "postgres://mini@"+addr+"/default?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = conn.Close(ctx)
	}()

	rows, err := conn.Query(ctx, "select 1 as i, 'hello' as t, 1.5 as f")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	if len(fields) != 3 {
		t.Fatalf("field count = %d, want 3", len(fields))
	}
	if fields[0].DataTypeOID != compat.OIDInt8 {
		t.Fatalf("int oid = %d, want %d", fields[0].DataTypeOID, compat.OIDInt8)
	}
	if fields[1].DataTypeOID != compat.OIDText {
		t.Fatalf("text oid = %d, want %d", fields[1].DataTypeOID, compat.OIDText)
	}
	if fields[2].DataTypeOID != compat.OIDFloat8 {
		t.Fatalf("float oid = %d, want %d", fields[2].DataTypeOID, compat.OIDFloat8)
	}

	var i int64
	var text string
	var f float64
	if !rows.Next() {
		t.Fatal("no rows")
	}
	if err := rows.Scan(&i, &text, &f); err != nil {
		t.Fatal(err)
	}
	if i != 1 || text != "hello" || f != 1.5 {
		t.Fatalf("row = (%d, %q, %v), want (1, hello, 1.5)", i, text, f)
	}
}

func TestPGXExtendedQueryWithParameter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	addr, stop := startTestServer(t, ctx)
	defer stop()

	conn, err := pgx.Connect(ctx, "postgres://mini@"+addr+"/default?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = conn.Close(ctx)
	}()

	var got string
	if err := conn.QueryRow(ctx, "select $1::text", "hello").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}

func TestPGXExtendedQueryWithDMLParameter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	addr, stop := startTestServer(t, ctx)
	defer stop()

	conn, err := pgx.Connect(ctx, "postgres://mini@"+addr+"/default?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = conn.Close(ctx)
	}()

	if _, err := conn.Exec(ctx, "create table users(id integer primary key, name text)"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "insert into users values ($1, $2)", 1, "alice"); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := conn.QueryRow(ctx, "select name from users where id = $1", 1).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "alice" {
		t.Fatalf("name = %q, want alice", name)
	}
}

func TestPGXTransactionRollbackAndCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	addr, stop := startTestServer(t, ctx)
	defer stop()

	conn, err := pgx.Connect(ctx, "postgres://mini@"+addr+"/default?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = conn.Close(ctx)
	}()

	if _, err := conn.Exec(ctx, "create table users(id integer primary key, name text)"); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "insert into users values (1, 'alice')"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := conn.QueryRow(ctx, "select count(*) from users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after rollback = %d, want 0", count)
	}

	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "insert into users values (1, 'alice')"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, "select count(*) from users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count after commit = %d, want 1", count)
	}
}

func TestPGXInitSQL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	addr, stop := startTestServerWithInit(t, ctx, "create table users(id integer primary key, name text); insert into users values (1, 'alice');")
	defer stop()

	conn, err := pgx.Connect(ctx, "postgres://mini@"+addr+"/default?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = conn.Close(ctx)
	}()

	var name string
	if err := conn.QueryRow(ctx, "select name from users where id = 1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "alice" {
		t.Fatalf("name = %q, want alice", name)
	}
}

func startTestServer(t *testing.T, ctx context.Context) (string, func()) {
	return startTestServerWithInit(t, ctx, "")
}

func startTestServerWithInit(t *testing.T, ctx context.Context, initSQL string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	cat, err := catalog.New(ctx, initSQL)
	if err != nil {
		t.Fatal(err)
	}
	handler := pgwire.NewHandler(cat)
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handler.ServeConn(ctx, conn)
		}
	}()
	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
		cat.Close(ctx)
	}
}
