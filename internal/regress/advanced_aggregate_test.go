package regress_test

import "testing"

func TestAggregateDistinctAdvanced(t *testing.T) {
	run(t, testGroup{
		Name: "AGGREGATE_DISTINCT",
		Setup: []string{
			"CREATE TABLE events (id INTEGER PRIMARY KEY, category TEXT, value INTEGER)",
			"INSERT INTO events VALUES (1, 'a', 10), (2, 'b', 20), (3, 'a', 10), (4, 'c', 30), (5, 'b', 20), (6, 'a', 15)",
		},
		Cases: map[string]testCase{
			"count_distinct": {
				SQL:  "SELECT COUNT(DISTINCT category) FROM events",
				Want: [][]string{{"3"}},
			},
			"sum_distinct": {
				SQL:  "SELECT SUM(DISTINCT value) FROM events",
				Want: [][]string{{"75"}},
			},
			"count_distinct_with_group": {
				SQL:  "SELECT category, COUNT(DISTINCT value) AS unique_vals FROM events GROUP BY category ORDER BY category",
				Want: [][]string{{"a", "2"}, {"b", "1"}, {"c", "1"}},
			},
		},
	})
}

func TestHavingComplex(t *testing.T) {
	run(t, testGroup{
		Name: "HAVING_COMPLEX",
		Setup: []string{
			"CREATE TABLE staff (id INTEGER PRIMARY KEY, dept TEXT, salary INTEGER, active INTEGER)",
			"INSERT INTO staff VALUES (1, 'eng', 80000, 1), (2, 'eng', 90000, 1), (3, 'eng', 70000, 0), (4, 'sales', 75000, 1), (5, 'sales', 85000, 1), (6, 'hr', 60000, 1)",
		},
		Cases: map[string]testCase{
			"having_multiple_conditions": {
				SQL:  "SELECT dept, COUNT(*) AS cnt, AVG(salary) AS avg_sal FROM staff GROUP BY dept HAVING COUNT(*) > 1 AND AVG(salary) > 75000 ORDER BY dept",
				Want: [][]string{{"eng", "3", "80000"}, {"sales", "2", "80000"}},
			},
			"having_with_sum": {
				SQL:  "SELECT dept, SUM(salary) AS total FROM staff WHERE active = 1 GROUP BY dept HAVING SUM(salary) > 100000 ORDER BY dept",
				Want: [][]string{{"eng", "170000"}, {"sales", "160000"}},
			},
			"having_or": {
				SQL:  "SELECT dept, COUNT(*) AS cnt FROM staff GROUP BY dept HAVING COUNT(*) >= 3 OR MIN(salary) < 65000 ORDER BY dept",
				Want: [][]string{{"eng", "3"}, {"hr", "1"}},
			},
		},
	})
}

func TestAggregateFilter(t *testing.T) {
	run(t, testGroup{
		Name: "AGGREGATE_FILTER",
		Setup: []string{
			"CREATE TABLE metrics (id INTEGER PRIMARY KEY, category TEXT, value INTEGER)",
			"INSERT INTO metrics VALUES (1, 'a', 10), (2, 'b', 20), (3, 'a', 30), (4, 'b', 40), (5, 'a', 50)",
		},
		Cases: map[string]testCase{
			"count_filter": {
				SQL:  "SELECT COUNT(*) FILTER (WHERE category = 'a') AS a_count FROM metrics",
				Want: [][]string{{"3"}},
			},
			"sum_filter": {
				SQL:  "SELECT SUM(value) FILTER (WHERE value > 20) AS big_sum FROM metrics",
				Want: [][]string{{"120"}},
			},
			"multiple_filters": {
				SQL:  "SELECT COUNT(*) FILTER (WHERE category = 'a') AS a_cnt, COUNT(*) FILTER (WHERE category = 'b') AS b_cnt FROM metrics",
				Want: [][]string{{"3", "2"}},
			},
		},
	})
}
