package regress_test

import "testing"

func TestTypeInteger(t *testing.T) {
	run(t, testGroup{
		Name: "integer",
		Setup: []string{
			"CREATE TABLE t(a INTEGER, b INTEGER)",
			"INSERT INTO t VALUES (1, -2147483648)",
			"INSERT INTO t VALUES (2, 2147483647)",
		},
		Cases: map[string]testCase{
			"select_values": {
				SQL:  "SELECT b FROM t ORDER BY a",
				Want: [][]string{{"-2147483648"}, {"2147483647"}},
			},
			"integer_literal": {
				SQL:  "SELECT 42",
				Want: [][]string{{"42"}},
			},
		},
	})
}

func TestTypeText(t *testing.T) {
	run(t, testGroup{
		Name: "text",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, s TEXT)",
			"INSERT INTO t VALUES (1, 'hello')",
			"INSERT INTO t VALUES (2, '')",
			"INSERT INTO t VALUES (3, 'it''s')",
		},
		Cases: map[string]testCase{
			"normal_string": {
				SQL:  "SELECT s FROM t WHERE id = 1",
				Want: [][]string{{"hello"}},
			},
			"empty_string": {
				SQL:  "SELECT s FROM t WHERE id = 2",
				Want: [][]string{{""}},
			},
			"escaped_quote": {
				SQL:  "SELECT s FROM t WHERE id = 3",
				Want: [][]string{{"it's"}},
			},
			"string_literal": {
				SQL:  "SELECT 'test'",
				Want: [][]string{{"test"}},
			},
		},
	})
}

func TestTypeBoolean(t *testing.T) {
	run(t, testGroup{
		Name: "boolean",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, flag BOOLEAN)",
			"INSERT INTO t VALUES (1, true)",
			"INSERT INTO t VALUES (2, false)",
		},
		Cases: map[string]testCase{
			"true_value": {
				SQL:  "SELECT flag FROM t WHERE id = 1",
				Want: [][]string{{"1"}},
			},
			"false_value": {
				SQL:  "SELECT flag FROM t WHERE id = 2",
				Want: [][]string{{"0"}},
			},
			"boolean_literal_true": {
				SQL:  "SELECT true",
				Want: [][]string{{"1"}},
				Skip: "boolean literal rewrite not implemented",
			},
			"boolean_literal_false": {
				SQL:  "SELECT false",
				Want: [][]string{{"0"}},
				Skip: "boolean literal rewrite not implemented",
			},
		},
	})
}

func TestTypeNull(t *testing.T) {
	run(t, testGroup{
		Name: "null",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, val TEXT)",
			"INSERT INTO t VALUES (1, NULL)",
			"INSERT INTO t VALUES (2, 'data')",
		},
		Cases: map[string]testCase{
			"select_null": {
				SQL:  "SELECT val FROM t WHERE id = 1",
				Want: [][]string{{"<nil>"}},
			},
			"null_literal": {
				SQL:  "SELECT NULL",
				Want: [][]string{{"<nil>"}},
			},
			"is_null": {
				SQL:  "SELECT id FROM t WHERE val IS NULL",
				Want: [][]string{{"1"}},
			},
			"is_not_null": {
				SQL:  "SELECT id FROM t WHERE val IS NOT NULL",
				Want: [][]string{{"2"}},
			},
		},
	})
}

func TestTypeReal(t *testing.T) {
	run(t, testGroup{
		Name: "real",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, val REAL)",
			"INSERT INTO t VALUES (1, 3.14)",
			"INSERT INTO t VALUES (2, -0.5)",
		},
		Cases: map[string]testCase{
			"positive": {
				SQL:  "SELECT val FROM t WHERE id = 1",
				Want: [][]string{{"3.14"}},
			},
			"negative": {
				SQL:  "SELECT val FROM t WHERE id = 2",
				Want: [][]string{{"-0.5"}},
			},
			"float_literal": {
				SQL:  "SELECT 1.5",
				Want: [][]string{{"1.5"}},
			},
		},
	})
}

func TestTypeSerial(t *testing.T) {
	run(t, testGroup{
		Name: "serial",
		Setup: []string{
			"CREATE TABLE t(id SERIAL PRIMARY KEY, name TEXT)",
			"INSERT INTO t(name) VALUES ('alice')",
			"INSERT INTO t(name) VALUES ('bob')",
		},
		Cases: map[string]testCase{
			"auto_increment": {
				SQL:  "SELECT id, name FROM t ORDER BY id",
				Want: [][]string{{"1", "alice"}, {"2", "bob"}},
				Skip: "SERIAL auto-increment behavior may differ",
			},
		},
	})
}
