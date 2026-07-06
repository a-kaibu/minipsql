package compat

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitStatementsSkipsComments(t *testing.T) {
	sql := `
SELECT 'before';
/* outer
   /* nested */
*/
SELECT -- line comment
  'after';
/* final comment */
`
	got := SplitStatements(sql)
	want := []string{
		"SELECT 'before'",
		"SELECT \n  'after'",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitStatements() = %#v, want %#v", got, want)
	}
}

func TestSplitStatementsKeepsDollarQuotedBlockTogether(t *testing.T) {
	sql := `DO $doblock$
begin
  perform 1;
end
$doblock$;
SELECT 1;`
	got := SplitStatements(sql)
	want := []string{
		`DO $doblock$
begin
  perform 1;
end
$doblock$`,
		"SELECT 1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitStatements() = %#v, want %#v", got, want)
	}
}

func TestRewritePositionIn(t *testing.T) {
	sql := "SELECT POSITION(convert_from('\\xbcf6', 'EUC_KR') IN\n  convert_from('\\xb0fa\\xbcf6', 'EUC_KR'))"
	want := `SELECT instr(convert_from('\xb0fa\xbcf6', 'EUC_KR'), convert_from('\xbcf6', 'EUC_KR'))`
	if got := Rewrite(sql); got != want {
		t.Fatalf("Rewrite() = %q, want %q", got, want)
	}
}

func TestRewriteCharLiteral(t *testing.T) {
	sql := "SELECT char 'c' = char 'c'"
	want := "SELECT 'c' = 'c'"
	if got := Rewrite(sql); got != want {
		t.Fatalf("Rewrite() = %q, want %q", got, want)
	}
}

func TestRewriteQuotedCharCast(t *testing.T) {
	sql := `SELECT 'a'::"char"::text`
	want := "SELECT CAST(CAST('a' AS text) AS text)"
	if got := Rewrite(sql); got != want {
		t.Fatalf("Rewrite() = %q, want %q", got, want)
	}
}

func TestRewriteTypedLiteralsAndCreateIndexUsing(t *testing.T) {
	cases := map[string]string{
		`SELECT pg_lsn '0/16AE7F7'`:                                                     `SELECT '0/16AE7F7'`,
		`INSERT INTO line_tbl VALUES (line(point '(3,1)', point '(3,2)'))`:              `INSERT INTO line_tbl VALUES (line('(3,1)', '(3,2)'))`,
		`CREATE INDEX macaddr_data_btree ON macaddr_data USING btree (b)`:               `CREATE INDEX macaddr_data_btree ON macaddr_data (b)`,
		`SELECT '[{"attributes": [], "dependency": 2, "degree": 1}]' ::pg_dependencies`: `SELECT CAST('[{"attributes": [], "dependency": 2, "degree": 1}]' AS text)`,
		`SELECT b::macaddr <> '08:00:2b:01:02:04'::macaddr FROM macaddr8_data`:          `SELECT CAST(b AS text)<> CAST('08:00:2b:01:02:04' AS text) FROM macaddr8_data`,
		`SELECT (i || '/' || j)::pg_lsn f FROM t ORDER BY f`:                            `SELECT CAST((i || '/' || j) AS text) f FROM t ORDER BY f`,
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteGenerateSeriesBareAliases(t *testing.T) {
	sql := `SELECT DISTINCT (i || '/' || j)::pg_lsn f FROM generate_series(1, 10) i, generate_series(1, 10) j WHERE i <= 10 ORDER BY f`
	got := Rewrite(sql)
	if strings.Contains(got, "generate_series") {
		t.Fatalf("Rewrite() still contains generate_series: %q", got)
	}
	if strings.Contains(got, ") i,") || strings.Contains(got, ") j ") {
		t.Fatalf("Rewrite() left bare aliases behind: %q", got)
	}
	if !strings.Contains(got, "AS i") || !strings.Contains(got, "AS j") {
		t.Fatalf("Rewrite() missing aliases: %q", got)
	}
	if !strings.Contains(got, "_gs_cte(i)") || !strings.Contains(got, "_gs_cte(j)") {
		t.Fatalf("Rewrite() missing bare-alias column names: %q", got)
	}
}

func TestRewritePositionEdgeCases(t *testing.T) {
	cases := map[string]string{
		"SELECT my_position, POSITION('a' IN 'abc')":                "SELECT my_position, instr('abc', 'a')",
		"SELECT POSITION('a' IN 'abc'), my_position":                "SELECT instr('abc', 'a'), my_position",
		"SELECT POSITION(x":                                         "SELECT POSITION(x",
		"SELECT POSITION('a' IN 'abc'), POSITION('x' IN my_position)": "SELECT instr('abc', 'a'), instr(my_position, 'x')",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteDeleteAlias(t *testing.T) {
	cases := map[string]string{
		"delete from delete_test dt where delete_test.a > 0": "delete from delete_test AS dt where dt.a > 0",
		"DELETE FROM schema.table t WHERE schema.table.id = 1": "DELETE FROM schema.table AS t WHERE t.id = 1",
		"DELETE FROM my_table WHERE id = 1": "DELETE FROM my_table WHERE id = 1",
		"DELETE FROM my_table AS t WHERE t.id = 1": "DELETE FROM my_table AS t WHERE t.id = 1",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteScientificNotationCast(t *testing.T) {
	cases := map[string]string{
		"SELECT 1.175494e-38::float4": "SELECT CAST(1.175494e-38 AS real)",
		"SELECT 1.23e+4::numeric":      "SELECT CAST(1.23e+4 AS numeric)",
		"SELECT -1e-3::real":           "SELECT -CAST(1e-3 AS real)",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteAnonymousIndex(t *testing.T) {
	got := Rewrite("CREATE INDEX ON my_table USING btree (col)")
	if !strings.Contains(got, "CREATE INDEX idx_anon_") || !strings.Contains(got, "ON my_table (col)") {
		t.Fatalf("unexpected anonymous index rewrite: %q", got)
	}
	got2 := Rewrite("CREATE UNIQUE INDEX ON my_table (col)")
	if !strings.Contains(got2, "CREATE UNIQUE INDEX idx_anon_") || !strings.Contains(got2, "ON my_table (col)") {
		t.Fatalf("unexpected anonymous unique index rewrite: %q", got2)
	}
}

func TestRewriteDistanceOperator(t *testing.T) {
	cases := map[string]string{
		"SELECT c1.f1 <-> c2.f1":             "SELECT dist_op(c1.f1, c2.f1)",
		"SELECT f1 <-> '0,1'":                "SELECT dist_op(f1, '0,1')",
		"SELECT (c1.f1 <-> c2.f1) AS dist":  "SELECT (dist_op(c1.f1, c2.f1)) AS dist",
		"SELECT point(1,2) <-> point(3,4)":  "SELECT dist_op(point(1,2), point(3,4))",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteTypedLiterals(t *testing.T) {
	cases := map[string]string{
		"SELECT int2 '2'":                "SELECT CAST('2' AS integer)",
		"SELECT int4 '10' + int8 '20'":   "SELECT CAST('10' AS integer) + CAST('20' AS integer)",
		"SELECT numeric '1.5'":           "SELECT CAST('1.5' AS numeric)",
		"SELECT timestamp '2020-01-01'": "SELECT CAST('2020-01-01' AS text)",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}
