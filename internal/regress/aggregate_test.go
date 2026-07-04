package regress_test

import "testing"

func TestAggregateBasic(t *testing.T) {
	run(t, testGroup{
		Name: "basic",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, value INTEGER)",
			"INSERT INTO t VALUES (1, 10), (2, 20), (3, 30), (4, NULL)",
		},
		Cases: map[string]testCase{
			"count_star": {
				SQL:  "SELECT COUNT(*) FROM t",
				Want: [][]string{{"4"}},
			},
			"count_column": {
				SQL:  "SELECT COUNT(value) FROM t",
				Want: [][]string{{"3"}},
			},
			"sum": {
				SQL:  "SELECT SUM(value) FROM t",
				Want: [][]string{{"60"}},
			},
			"avg": {
				SQL:  "SELECT AVG(value) FROM t",
				Want: [][]string{{"20"}},
			},
			"min": {
				SQL:  "SELECT MIN(value) FROM t",
				Want: [][]string{{"10"}},
			},
			"max": {
				SQL:  "SELECT MAX(value) FROM t",
				Want: [][]string{{"30"}},
			},
		},
	})
}

func TestAggregateEmpty(t *testing.T) {
	run(t, testGroup{
		Name: "empty_table",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, value INTEGER)",
		},
		Cases: map[string]testCase{
			"count_star_zero": {
				SQL:  "SELECT COUNT(*) FROM t",
				Want: [][]string{{"0"}},
			},
			"sum_null": {
				SQL:  "SELECT SUM(value) FROM t",
				Want: [][]string{{"<nil>"}},
			},
			"avg_null": {
				SQL:  "SELECT AVG(value) FROM t",
				Want: [][]string{{"<nil>"}},
			},
			"min_null": {
				SQL:  "SELECT MIN(value) FROM t",
				Want: [][]string{{"<nil>"}},
			},
			"max_null": {
				SQL:  "SELECT MAX(value) FROM t",
				Want: [][]string{{"<nil>"}},
			},
		},
	})
}

func TestAggregateGrouped(t *testing.T) {
	run(t, testGroup{
		Name: "grouped",
		Setup: []string{
			"CREATE TABLE t(category TEXT, value INTEGER)",
			"INSERT INTO t VALUES ('a', 1), ('a', 2), ('b', 10), ('b', 20)",
		},
		Cases: map[string]testCase{
			"count_per_group": {
				SQL:  "SELECT category, COUNT(*) FROM t GROUP BY category ORDER BY category",
				Want: [][]string{{"a", "2"}, {"b", "2"}},
			},
			"sum_per_group": {
				SQL:  "SELECT category, SUM(value) FROM t GROUP BY category ORDER BY category",
				Want: [][]string{{"a", "3"}, {"b", "30"}},
			},
			"having_filter": {
				SQL:  "SELECT category, SUM(value) FROM t GROUP BY category HAVING SUM(value) > 5",
				Want: [][]string{{"b", "30"}},
			},
		},
	})
}
