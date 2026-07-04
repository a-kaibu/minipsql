package regress_test

import "testing"

func TestSelectWhere(t *testing.T) {
	run(t, testGroup{
		Name: "conditions",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT, age INTEGER)",
			"INSERT INTO t VALUES (1, 'alice', 30)",
			"INSERT INTO t VALUES (2, 'bob', 25)",
			"INSERT INTO t VALUES (3, 'carol', 30)",
		},
		Cases: map[string]testCase{
			"equality": {
				SQL:  "SELECT name FROM t WHERE id = 1",
				Want: [][]string{{"alice"}},
			},
			"not_equal": {
				SQL:  "SELECT name FROM t WHERE id != 1 ORDER BY id",
				Want: [][]string{{"bob"}, {"carol"}},
			},
			"greater_than": {
				SQL:  "SELECT name FROM t WHERE id > 1 ORDER BY id",
				Want: [][]string{{"bob"}, {"carol"}},
			},
			"less_than": {
				SQL:  "SELECT name FROM t WHERE id < 2",
				Want: [][]string{{"alice"}},
			},
			"and": {
				SQL:  "SELECT name FROM t WHERE age = 30 AND id = 3",
				Want: [][]string{{"carol"}},
			},
			"or": {
				SQL:  "SELECT name FROM t WHERE id = 1 OR id = 3 ORDER BY id",
				Want: [][]string{{"alice"}, {"carol"}},
			},
			"not": {
				SQL:  "SELECT name FROM t WHERE NOT id = 1 ORDER BY id",
				Want: [][]string{{"bob"}, {"carol"}},
			},
		},
	})
}

func TestSelectOrderBy(t *testing.T) {
	run(t, testGroup{
		Name: "ordering",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT)",
			"INSERT INTO t VALUES (3, 'carol')",
			"INSERT INTO t VALUES (1, 'alice')",
			"INSERT INTO t VALUES (2, 'bob')",
		},
		Cases: map[string]testCase{
			"asc": {
				SQL:  "SELECT name FROM t ORDER BY id ASC",
				Want: [][]string{{"alice"}, {"bob"}, {"carol"}},
			},
			"desc": {
				SQL:  "SELECT name FROM t ORDER BY id DESC",
				Want: [][]string{{"carol"}, {"bob"}, {"alice"}},
			},
			"by_name": {
				SQL:  "SELECT name FROM t ORDER BY name",
				Want: [][]string{{"alice"}, {"bob"}, {"carol"}},
			},
			"multi_column": {
				SQL:  "SELECT name FROM t ORDER BY id DESC, name ASC",
				Want: [][]string{{"carol"}, {"bob"}, {"alice"}},
			},
		},
	})
}

func TestSelectLimitOffset(t *testing.T) {
	run(t, testGroup{
		Name: "pagination",
		Setup: []string{
			"CREATE TABLE t(id INTEGER)",
			"INSERT INTO t VALUES (1)",
			"INSERT INTO t VALUES (2)",
			"INSERT INTO t VALUES (3)",
			"INSERT INTO t VALUES (4)",
			"INSERT INTO t VALUES (5)",
		},
		Cases: map[string]testCase{
			"limit": {
				SQL:  "SELECT id FROM t ORDER BY id LIMIT 3",
				Want: [][]string{{"1"}, {"2"}, {"3"}},
			},
			"limit_offset": {
				SQL:  "SELECT id FROM t ORDER BY id LIMIT 2 OFFSET 2",
				Want: [][]string{{"3"}, {"4"}},
			},
			"offset_with_limit_minus_one": {
				SQL:  "SELECT id FROM t ORDER BY id LIMIT -1 OFFSET 3",
				Want: [][]string{{"4"}, {"5"}},
			},
			"limit_zero": {
				SQL:  "SELECT id FROM t ORDER BY id LIMIT 0",
				Want: [][]string{},
			},
		},
	})
}

func TestSelectGroupBy(t *testing.T) {
	run(t, testGroup{
		Name: "grouping",
		Setup: []string{
			"CREATE TABLE sales(product TEXT, amount INTEGER)",
			"INSERT INTO sales VALUES ('a', 10)",
			"INSERT INTO sales VALUES ('a', 20)",
			"INSERT INTO sales VALUES ('b', 5)",
			"INSERT INTO sales VALUES ('b', 15)",
		},
		Cases: map[string]testCase{
			"count": {
				SQL:  "SELECT product, COUNT(*) FROM sales GROUP BY product ORDER BY product",
				Want: [][]string{{"a", "2"}, {"b", "2"}},
			},
			"sum": {
				SQL:  "SELECT product, SUM(amount) FROM sales GROUP BY product ORDER BY product",
				Want: [][]string{{"a", "30"}, {"b", "20"}},
			},
			"having": {
				SQL:  "SELECT product, SUM(amount) FROM sales GROUP BY product HAVING SUM(amount) > 20 ORDER BY product",
				Want: [][]string{{"a", "30"}},
			},
		},
	})
}

func TestSelectDistinct(t *testing.T) {
	run(t, testGroup{
		Name: "distinct",
		Setup: []string{
			"CREATE TABLE t(category TEXT, value INTEGER)",
			"INSERT INTO t VALUES ('a', 1)",
			"INSERT INTO t VALUES ('a', 2)",
			"INSERT INTO t VALUES ('b', 1)",
		},
		Cases: map[string]testCase{
			"single_column": {
				SQL:  "SELECT DISTINCT category FROM t ORDER BY category",
				Want: [][]string{{"a"}, {"b"}},
			},
			"multi_column": {
				SQL:  "SELECT DISTINCT category, value FROM t ORDER BY category, value",
				Want: [][]string{{"a", "1"}, {"a", "2"}, {"b", "1"}},
			},
		},
	})
}

func TestSelectAlias(t *testing.T) {
	run(t, testGroup{
		Name: "alias",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT)",
			"INSERT INTO t VALUES (1, 'alice')",
		},
		Cases: map[string]testCase{
			"column_alias": {
				SQL:  "SELECT id AS user_id, name AS user_name FROM t",
				Want: [][]string{{"1", "alice"}},
			},
			"expression_alias": {
				SQL:  "SELECT id + 10 AS bumped FROM t",
				Want: [][]string{{"11"}},
			},
		},
	})
}
