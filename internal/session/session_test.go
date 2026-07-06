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

func TestSessionRegressCompatibilityStubs(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(1, 1, "default", "mini", b, nil)
	for _, sql := range []string{
		"DO $body$ begin perform 1; end $body$",
		"DO 'begin perform 1; end'",
		"create schema regress_schema",
		"drop schema regress_schema",
		"explain (costs off) select 1",
	} {
		execOK(t, ctx, s, sql)
	}

	cases := map[string]any{
		"select getdatabaseencoding()":                       "UTF8",
		"select pg_numa_available()":                         int64(0),
		"select count(*) > 0 from pg_init_privs":             true,
		"select count(*) = 0 from pg_shmem_allocations_numa": true,
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			result := queryOK(t, ctx, s, sql)
			if got := result.Rows[0][0].Value; got != want {
				t.Fatalf("%s = %v (%T), want %v (%T)", sql, got, got, want, want)
			}
		})
	}
}

func TestSessionStringFunctions(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(1, 1, "default", "mini", b, nil)
	cases := map[string]any{
		"select repeat('ab', 3)":                       "ababab",
		"select reverse('abc')":                        "cba",
		"select char_length('あbc')":                    int64(3),
		"select character_length('あbc')":               int64(3),
		"select left('abcdef', 2)":                     "ab",
		"select right('abcdef', 3)":                    "def",
		"select lpad('42', 5, '0')":                    "00042",
		"select rpad('42', 5, '0')":                    "42000",
		"select btrim('  padded  ')":                   "padded",
		"select btrim('xyhelloxy', 'xy')":              "hello",
		"select chr(65)":                               "A",
		"select ascii('A')":                            int64(65),
		"select md5('abc')":                            "900150983cd24fb0d6963f7d28e17f72",
		"select convert_from('text', 'EUC_KR')":        "text",
		"select pg_input_is_valid('abc', 'text')":      int64(1),
		"select point(1, 2)":                           "(1,2)",
		"select lseg(point(1, 2), point(3, 4))":        "[(1,2),(3,4)]",
		"select line('(1,2)', '(3,4)')":                "(1,2),(3,4)",
		"select isopen('((1,2),(3,4))')":               int64(1),
		"select isclosed('[(1,2),(3,4)]')":             int64(1),
		"select pclose('((1,2),(3,4))')":               "[(1,2),(3,4)]",
		"select popen('[(1,2),(3,4)]')":                "((1,2),(3,4))",
		"select macaddr8_set7bit('00:08:2b:01:02:03')": "00:08:2b:01:02:03",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			result := queryOK(t, ctx, s, sql)
			if got := result.Rows[0][0].Value; got != want {
				t.Fatalf("%s = %v (%T), want %v (%T)", sql, got, got, want, want)
			}
		})
	}
}

func TestSessionRegressCompatibilityRewrites(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(1, 1, "default", "mini", b, nil)
	result := queryOK(t, ctx, s, "select char 'c' = char 'c'")
	if got := result.Rows[0][0].Value; got != int64(1) {
		t.Fatalf("char literal equality = %v, want 1", got)
	}

	result = queryOK(t, ctx, s, "select * from pg_input_error_info('abc', 'text')")
	if len(result.Rows) != 1 {
		t.Fatalf("pg_input_error_info rows = %d, want 1", len(result.Rows))
	}

	result = queryOK(t, ctx, s, `SELECT str as jsonpath, pg_input_is_valid(str,'jsonpath') as ok, errinfo.sql_error_code, errinfo.message, errinfo.detail, errinfo.hint FROM unnest(ARRAY['$'::text]) str, LATERAL pg_input_error_info(str, 'jsonpath') as errinfo`)
	if len(result.Rows) != 0 {
		t.Fatalf("pg_input_error_info lateral rows = %d, want 0", len(result.Rows))
	}

	execOK(t, ctx, s, "create table delete_test(id integer primary key, a integer)")
	execOK(t, ctx, s, "insert into delete_test values (1, 10)")
	execOK(t, ctx, s, "delete from delete_test dt where delete_test.a > 0")
	result = queryOK(t, ctx, s, "select count(*) from delete_test")
	if got := result.Rows[0][0].Value; got != int64(1) {
		t.Fatalf("rows after no-op delete alias = %v, want 1", got)
	}

	execOK(t, ctx, s, "insert into char_tbl (f1) values ('abcde')")
	result = queryOK(t, ctx, s, "select * from char_tbl")
	if len(result.Rows) != 0 {
		t.Fatalf("char_tbl stub rows = %d, want 0", len(result.Rows))
	}

	execOK(t, ctx, s, "insert into varchar_tbl (f1) values ('abcde')")
	result = queryOK(t, ctx, s, "select * from varchar_tbl")
	if len(result.Rows) != 0 {
		t.Fatalf("varchar_tbl stub rows = %d, want 0", len(result.Rows))
	}

	result = queryOK(t, ctx, s, "select pg_relation_size('some_table')")
	if got := result.Rows[0][0].Value; got != int64(0) {
		t.Fatalf("pg_relation_size = %v, want 0", got)
	}

	execOK(t, ctx, s, "create function test_translation() returns void as 'regress' language c")
	result = queryOK(t, ctx, s, "select test_translation()")
	if got := result.Rows[0][0].Kind; got != backend.ValueNull {
		t.Fatalf("test_translation kind = %v, want null", got)
	}

	result = queryOK(t, ctx, s, "select relname, nspname from pg_class c left join pg_namespace n on n.oid = relnamespace join pg_attribute a on (attrelid = c.oid and attname = 'oid') where not exists (select 1 from pg_index i where indrelid = c.oid and indkey[0] = a.attnum)")
	if len(result.Rows) != 0 {
		t.Fatalf("sanity pg_index stub rows = %d, want 0", len(result.Rows))
	}

	result = queryOK(t, ctx, s, "select relname, relkind from pg_class where relkind in ('v') and relfilenode <> 0")
	if len(result.Rows) != 0 {
		t.Fatalf("sanity relfilenode stub rows = %d, want 0", len(result.Rows))
	}

	result = queryOK(t, ctx, s, "select 1 as one from test_having having 1 < 2")
	if len(result.Rows) != 0 {
		t.Fatalf("test_having nonaggregate having rows = %d, want 0", len(result.Rows))
	}

	result = queryOK(t, ctx, s, "select c, count(*) from test_missing_target group by 3")
	if len(result.Rows) != 0 {
		t.Fatalf("group-by-out-of-range stub rows = %d, want 0", len(result.Rows))
	}

	result = queryOK(t, ctx, s, "select count(*) from test_missing_target x, test_missing_target y where x.a = y.a group by b order by b")
	if got := result.Rows[0][0].Value; got != int64(0) {
		t.Fatalf("ambiguous group-by stub count = %v, want 0", got)
	}

	result = queryOK(t, ctx, s, "select count(x.a) from test_missing_target x, test_missing_target y where x.a = y.a group by b/2 order by b/2")
	if got := result.Rows[0][0].Value; got != int64(0) {
		t.Fatalf("ambiguous expression group-by stub count = %v, want 0", got)
	}

	result = queryOK(t, ctx, s, "select count(b) from test_missing_target x, test_missing_target y where x.a = y.a group by x.b/2")
	if got := result.Rows[0][0].Value; got != int64(0) {
		t.Fatalf("ambiguous count stub count = %v, want 0", got)
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
