package compat

import (
	"reflect"
	"regexp"
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
		`CREATE INDEX macaddr_data_btree ON macaddr_data USING btree (b)`:               `CREATE INDEX IF NOT EXISTS macaddr_data_btree ON macaddr_data (b)`,
		`SELECT '[{"attributes": [], "dependency": 2, "degree": 1}]' ::pg_dependencies`: `SELECT CAST('[{"attributes": [], "dependency": 2, "degree": 1}]' AS text)`,
		`SELECT b::macaddr <> '08:00:2b:01:02:04'::macaddr FROM macaddr8_data`:          `SELECT CAST(b AS text) <> CAST('08:00:2b:01:02:04' AS text) FROM macaddr8_data`,
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
	if !regexp.MustCompile(`_gs_cte_\d+\(i\)`).MatchString(got) || !regexp.MustCompile(`_gs_cte_\d+\(j\)`).MatchString(got) {
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
		"DELETE FROM schema.table t WHERE schema.table.id = 1": "DELETE FROM table AS t WHERE t.id = 1",
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

func TestRewriteDollarQuotes(t *testing.T) {
	cases := map[string]string{
		"SELECT $$hello$$":                         "SELECT 'hello'",
		"SELECT $tag$hello 'world'$tag$":           "SELECT 'hello ''world'''",
		"SELECT $re$(bar)(beque)$re$":               "SELECT '(bar)(beque)'",
		"SELECT $a-b$test$a-b$":                     "SELECT 'test'",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteRegexFunctions(t *testing.T) {
	cases := map[string]string{
		"SELECT foo FROM regexp_split_to_table('the quick', '\\s+') AS foo": 
			"SELECT foo FROM (SELECT value AS foo FROM json_each(minipsql_regexp_split_to_json('the quick', '\\s+'))) AS foo",
		
		"SELECT regexp_matches('foobarbequebaz', '(bar)(beque)')":
			"SELECT value FROM json_each(minipsql_regexp_matches_to_json('foobarbequebaz', '(bar)(beque)'))",
			
		"SELECT regexp_matches('foo', 'bar', 'g');":
			"SELECT value FROM json_each(minipsql_regexp_matches_to_json('foo', 'bar', 'g'));",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteOverlay(t *testing.T) {
	cases := map[string]string{
		"SELECT OVERLAY('abcdef' PLACING '45' FROM 4)": 
			"SELECT minipsql_overlay('abcdef', '45', 4)",
		
		"SELECT OVERLAY('yabadoo' PLACING 'daba' FROM 5 FOR 0)":
			"SELECT minipsql_overlay('yabadoo', 'daba', 5, 0)",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteStringContinuation(t *testing.T) {
	cases := map[string]string{
		"SELECT 'first line'\n' - next line'\n\t' - third line'":
			"SELECT 'first line - next line - third line'",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteToBinOctHex(t *testing.T) {
	cases := map[string]string{
		"SELECT to_bin(-1234)":                  "SELECT minipsql_to_bin_int(-1234)",
		"SELECT to_bin(-1234::bigint)":          "SELECT minipsql_to_bin_bigint(-CAST(1234 AS integer))",
		"SELECT to_oct(45::int8)":               "SELECT minipsql_to_oct_bigint(CAST(45 AS integer))",
		"SELECT to_hex(20)":                     "SELECT minipsql_to_hex_int(20)",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteSubstringAndTrim(t *testing.T) {
	cases := map[string]string{
		"SELECT SUBSTRING('1234567890'::bytea FROM 3)":               "SELECT substr(CAST('1234567890' AS blob), 3)",
		"SELECT SUBSTRING('1234567890'::bytea FROM 4 FOR 3)":         "SELECT substr(CAST('1234567890' AS blob), 4, 3)",
		"SELECT trim(leading E'\\\\000'::bytea from E'\\\\000'::bytea)": "SELECT ltrim(CAST('\\\\000' AS blob), CAST('\\\\000' AS blob))",
		"SELECT trim(trailing E'\\\\000'::bytea from E'\\\\000'::bytea)": "SELECT rtrim(CAST('\\\\000' AS blob), CAST('\\\\000' AS blob))",
		"SELECT trim(E'\\\\000'::bytea from E'\\\\000'::bytea)":         "SELECT btrim(CAST('\\\\000' AS blob), CAST('\\\\000' AS blob))",
		"SELECT SUBSTRING('abc', 1, 2)":                              "SELECT substr('abc', 1, 2)",
		"SELECT trim('abc', 'a')":                                    "SELECT btrim('abc', 'a')",
		"SELECT encode(overlay(E'Th\\\\000omas'::bytea placing E'Th\\\\001omas'::bytea from 2),'escape')": "SELECT encode(minipsql_overlay(CAST('Th\\\\000omas' AS blob), CAST('Th\\\\001omas' AS blob), 2),'escape')",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}

func TestRewriteUnicodeLiterals(t *testing.T) {
	cases := map[string]string{
		"SELECT U&'d\\0061t\\+000061'":                      "SELECT 'data'",
		"SELECT U&'d!0061t\\+000061' UESCAPE '!'":          "SELECT 'dat\\+000061'",
		"SELECT U&'a\\\\b' AS \"a\\b\"":                    "SELECT 'a\\b' AS \"a\\b\"",
		"SELECT U&'wrong: \\061'":                          "SELECT minipsql_error('invalid Unicode escape', '22025', 'Unicode escapes must be \\XXXX or \\+XXXXXX.')",
		"SELECT U&'wrong: +0061' UESCAPE +":                "SELECT minipsql_error('UESCAPE must be followed by a simple string literal at or near \"+\"', '42601', '')",
		"SELECT U&'wrong: +0061' UESCAPE '+'":              "SELECT minipsql_error('invalid Unicode escape character', '22025', '')",
		"SELECT CAST(name 'namefield' AS text)":            "SELECT CAST(CAST('namefield' AS text) AS text)",
		"SELECT SUBSTRING('abcdefg' SIMILAR 'a#\"(b_d)#\"%' ESCAPE '#')": "SELECT minipsql_similar_substring('abcdefg', 'a#\"(b_d)#\"%', '#')",
		"SELECT SUBSTRING('abcdefg' FROM 'a#\"(b_d)#\"%' FOR '#')":       "SELECT minipsql_similar_substring('abcdefg', 'a#\"(b_d)#\"%', '#')",
		"SELECT 'abcdefg' SIMILAR TO '_bcd#%' ESCAPE '#'":                "SELECT minipsql_similar_to('abcdefg', '_bcd#%', '#')",
		"SELECT 'abcdefg' SIMILAR TO '_bcd%'":                            "SELECT minipsql_similar_to('abcdefg', '_bcd%', '\\')",
		"SELECT * FROM (VALUES (-2.5::float8), (1.5::float8)) t(x)":      "SELECT * FROM (SELECT -CAST(2.5 AS real) AS x UNION ALL SELECT CAST(1.5 AS real)) t",
		"SELECT * FROM INT2_TBL AS f(a, b)":                              "SELECT * FROM INT2_TBL f",
		"SELECT * FROM (TABLE int2_tbl) AS s(a, b)":                      "SELECT * FROM (SELECT * FROM int2_tbl) AS s",
		"SELECT ARRAY[1, 2, 3]":                                          "SELECT minipsql_array(1, 2, 3)",
		"SELECT DISTINCT age FROM person* ORDER BY age using >":         "SELECT DISTINCT age FROM person ORDER BY age DESC",
		"(SELECT * FROM t1 EXCEPT SELECT * FROM t2)":                     "SELECT * FROM (SELECT * FROM t1 EXCEPT SELECT * FROM t2)",
	}
	for sql, want := range cases {
		t.Run(sql, func(t *testing.T) {
			if got := Rewrite(sql); got != want {
				t.Fatalf("Rewrite() = %q, want %q", got, want)
			}
		})
	}
}






func TestSplitDollarQuoteSemicolon(t *testing.T) {
	sql := "CREATE FUNCTION f() RETURNS int LANGUAGE SQL AS $$ SELECT 1; $$;\nSELECT f();"
	stmts := SplitStatements(sql)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
}
