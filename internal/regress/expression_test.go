package regress_test

import "testing"

func TestArithmetic(t *testing.T) {
	run(t, testGroup{
		Name: "arithmetic",
		Cases: map[string]testCase{
			"add":        {SQL: "SELECT 1 + 2", Want: [][]string{{"3"}}},
			"subtract":   {SQL: "SELECT 10 - 3", Want: [][]string{{"7"}}},
			"multiply":   {SQL: "SELECT 4 * 5", Want: [][]string{{"20"}}},
			"divide":     {SQL: "SELECT 10 / 3", Want: [][]string{{"3"}}},
			"modulo":     {SQL: "SELECT 10 % 3", Want: [][]string{{"1"}}},
			"negative":   {SQL: "SELECT -5", Want: [][]string{{"-5"}}},
			"precedence": {SQL: "SELECT 2 + 3 * 4", Want: [][]string{{"14"}}},
			"parens":     {SQL: "SELECT (2 + 3) * 4", Want: [][]string{{"20"}}},
		},
	})
}

func TestStringConcat(t *testing.T) {
	run(t, testGroup{
		Name: "concat",
		Cases: map[string]testCase{
			"pipe_operator": {SQL: "SELECT 'hello' || ' ' || 'world'", Want: [][]string{{"hello world"}}},
			"with_number":   {SQL: "SELECT 'id=' || 42", Want: [][]string{{"id=42"}}},
		},
	})
}

func TestCaseWhen(t *testing.T) {
	run(t, testGroup{
		Name: "case",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, status TEXT)",
			"INSERT INTO t VALUES (1, 'active'), (2, 'inactive'), (3, 'pending')",
		},
		Cases: map[string]testCase{
			"simple_case": {
				SQL:  "SELECT id, CASE status WHEN 'active' THEN 'on' WHEN 'inactive' THEN 'off' ELSE 'unknown' END FROM t ORDER BY id",
				Want: [][]string{{"1", "on"}, {"2", "off"}, {"3", "unknown"}},
			},
			"searched_case": {
				SQL:  "SELECT id, CASE WHEN id < 2 THEN 'low' WHEN id < 3 THEN 'mid' ELSE 'high' END FROM t ORDER BY id",
				Want: [][]string{{"1", "low"}, {"2", "mid"}, {"3", "high"}},
			},
			"case_no_else": {
				SQL:  "SELECT CASE WHEN 1 = 2 THEN 'yes' END",
				Want: [][]string{{"<nil>"}},
			},
		},
	})
}

func TestCoalesce(t *testing.T) {
	run(t, testGroup{
		Name: "coalesce",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT)",
			"INSERT INTO t VALUES (1, 'alice')",
			"INSERT INTO t VALUES (2, NULL)",
		},
		Cases: map[string]testCase{
			"non_null": {
				SQL:  "SELECT COALESCE(name, 'unknown') FROM t WHERE id = 1",
				Want: [][]string{{"alice"}},
			},
			"null_fallback": {
				SQL:  "SELECT COALESCE(name, 'unknown') FROM t WHERE id = 2",
				Want: [][]string{{"unknown"}},
			},
			"multiple_args": {
				SQL:  "SELECT COALESCE(NULL, NULL, 'third')",
				Want: [][]string{{"third"}},
			},
		},
	})
}

func TestNullif(t *testing.T) {
	run(t, testGroup{
		Name: "nullif",
		Cases: map[string]testCase{
			"equal_returns_null": {
				SQL:  "SELECT NULLIF(1, 1)",
				Want: [][]string{{"<nil>"}},
			},
			"not_equal_returns_first": {
				SQL:  "SELECT NULLIF(1, 2)",
				Want: [][]string{{"1"}},
			},
		},
	})
}
