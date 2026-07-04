package regress_test

import "testing"

func TestOperatorLike(t *testing.T) {
	run(t, testGroup{
		Name: "like",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT)",
			"INSERT INTO t VALUES (1, 'alice'), (2, 'bob'), (3, 'ali'), (4, 'ALICE')",
		},
		Cases: map[string]testCase{
			"prefix": {
				SQL:  "SELECT name FROM t WHERE name LIKE 'ali%' ORDER BY id",
				Want: [][]string{{"alice"}, {"ali"}, {"ALICE"}},
			},
			"suffix": {
				SQL:  "SELECT name FROM t WHERE name LIKE '%ce' ORDER BY id",
				Want: [][]string{{"alice"}, {"ALICE"}},
			},
			"contains": {
				SQL:  "SELECT name FROM t WHERE name LIKE '%li%' ORDER BY id",
				Want: [][]string{{"alice"}, {"ali"}, {"ALICE"}},
			},
			"single_char": {
				SQL:  "SELECT name FROM t WHERE name LIKE 'bo_' ORDER BY id",
				Want: [][]string{{"bob"}},
			},
			"not_like": {
				SQL:  "SELECT name FROM t WHERE name NOT LIKE 'ali%' ORDER BY id",
				Want: [][]string{{"bob"}},
			},
			"ilike": {
				SQL:  "SELECT name FROM t WHERE name ILIKE 'alice' ORDER BY id",
				Want: [][]string{{"alice"}, {"ALICE"}},
			},
		},
	})
}

func TestOperatorIn(t *testing.T) {
	run(t, testGroup{
		Name: "in",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT)",
			"INSERT INTO t VALUES (1, 'alice'), (2, 'bob'), (3, 'carol')",
		},
		Cases: map[string]testCase{
			"in_list": {
				SQL:  "SELECT name FROM t WHERE id IN (1, 3) ORDER BY id",
				Want: [][]string{{"alice"}, {"carol"}},
			},
			"not_in_list": {
				SQL:  "SELECT name FROM t WHERE id NOT IN (1, 3) ORDER BY id",
				Want: [][]string{{"bob"}},
			},
			"in_strings": {
				SQL:  "SELECT id FROM t WHERE name IN ('alice', 'bob') ORDER BY id",
				Want: [][]string{{"1"}, {"2"}},
			},
		},
	})
}

func TestOperatorBetween(t *testing.T) {
	run(t, testGroup{
		Name: "between",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, value INTEGER)",
			"INSERT INTO t VALUES (1, 10), (2, 20), (3, 30), (4, 40)",
		},
		Cases: map[string]testCase{
			"inclusive": {
				SQL:  "SELECT id FROM t WHERE value BETWEEN 20 AND 30 ORDER BY id",
				Want: [][]string{{"2"}, {"3"}},
			},
			"not_between": {
				SQL:  "SELECT id FROM t WHERE value NOT BETWEEN 20 AND 30 ORDER BY id",
				Want: [][]string{{"1"}, {"4"}},
			},
		},
	})
}

func TestOperatorComparison(t *testing.T) {
	run(t, testGroup{
		Name: "comparison",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, val INTEGER)",
			"INSERT INTO t VALUES (1, 10), (2, 20), (3, 30)",
		},
		Cases: map[string]testCase{
			"equal": {
				SQL:  "SELECT id FROM t WHERE val = 20",
				Want: [][]string{{"2"}},
			},
			"not_equal_ansi": {
				SQL:  "SELECT id FROM t WHERE val <> 20 ORDER BY id",
				Want: [][]string{{"1"}, {"3"}},
			},
			"not_equal_bang": {
				SQL:  "SELECT id FROM t WHERE val != 20 ORDER BY id",
				Want: [][]string{{"1"}, {"3"}},
			},
			"less": {
				SQL:  "SELECT id FROM t WHERE val < 20",
				Want: [][]string{{"1"}},
			},
			"less_or_equal": {
				SQL:  "SELECT id FROM t WHERE val <= 20 ORDER BY id",
				Want: [][]string{{"1"}, {"2"}},
			},
			"greater": {
				SQL:  "SELECT id FROM t WHERE val > 20",
				Want: [][]string{{"3"}},
			},
			"greater_or_equal": {
				SQL:  "SELECT id FROM t WHERE val >= 20 ORDER BY id",
				Want: [][]string{{"2"}, {"3"}},
			},
		},
	})
}

func TestOperatorIsNull(t *testing.T) {
	run(t, testGroup{
		Name: "null_ops",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, val TEXT)",
			"INSERT INTO t VALUES (1, 'a'), (2, NULL), (3, 'c')",
		},
		Cases: map[string]testCase{
			"is_null": {
				SQL:  "SELECT id FROM t WHERE val IS NULL",
				Want: [][]string{{"2"}},
			},
			"is_not_null": {
				SQL:  "SELECT id FROM t WHERE val IS NOT NULL ORDER BY id",
				Want: [][]string{{"1"}, {"3"}},
			},
			"null_equality_always_false": {
				SQL:  "SELECT id FROM t WHERE val = NULL",
				Want: [][]string{},
			},
		},
	})
}
