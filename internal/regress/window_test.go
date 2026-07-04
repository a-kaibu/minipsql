package regress_test

import "testing"

func TestWindowRowNumber(t *testing.T) {
	run(t, testGroup{
		Name: "WINDOW_ROW_NUMBER",
		Setup: []string{
			"CREATE TABLE sales (id INTEGER PRIMARY KEY, dept TEXT, amount INTEGER)",
			"INSERT INTO sales VALUES (1, 'eng', 100), (2, 'eng', 200), (3, 'eng', 150), (4, 'sales', 300), (5, 'sales', 250)",
		},
		Cases: map[string]testCase{
			"basic_row_number": {
				SQL:  "SELECT id, ROW_NUMBER() OVER (ORDER BY id) AS rn FROM sales ORDER BY id",
				Want: [][]string{{"1", "1"}, {"2", "2"}, {"3", "3"}, {"4", "4"}, {"5", "5"}},
			},
			"partition_row_number": {
				SQL:  "SELECT id, dept, ROW_NUMBER() OVER (PARTITION BY dept ORDER BY amount DESC) AS rn FROM sales ORDER BY dept, rn",
				Want: [][]string{{"2", "eng", "1"}, {"3", "eng", "2"}, {"1", "eng", "3"}, {"4", "sales", "1"}, {"5", "sales", "2"}},
			},
		},
	})
}

func TestWindowRank(t *testing.T) {
	run(t, testGroup{
		Name: "WINDOW_RANK",
		Setup: []string{
			"CREATE TABLE scores (id INTEGER PRIMARY KEY, name TEXT, score INTEGER)",
			"INSERT INTO scores VALUES (1, 'alice', 90), (2, 'bob', 85), (3, 'carol', 90), (4, 'dave', 80), (5, 'eve', 85)",
		},
		Cases: map[string]testCase{
			"rank": {
				SQL:  "SELECT name, RANK() OVER (ORDER BY score DESC) AS rnk FROM scores ORDER BY rnk, name",
				Want: [][]string{{"alice", "1"}, {"carol", "1"}, {"bob", "3"}, {"eve", "3"}, {"dave", "5"}},
			},
			"dense_rank": {
				SQL:  "SELECT name, DENSE_RANK() OVER (ORDER BY score DESC) AS drnk FROM scores ORDER BY drnk, name",
				Want: [][]string{{"alice", "1"}, {"carol", "1"}, {"bob", "2"}, {"eve", "2"}, {"dave", "3"}},
			},
		},
	})
}

func TestWindowLagLead(t *testing.T) {
	run(t, testGroup{
		Name: "WINDOW_LAG_LEAD",
		Setup: []string{
			"CREATE TABLE timeline (id INTEGER PRIMARY KEY, value INTEGER)",
			"INSERT INTO timeline VALUES (1, 10), (2, 20), (3, 15), (4, 30), (5, 25)",
		},
		Cases: map[string]testCase{
			"lag": {
				SQL:  "SELECT id, value, LAG(value) OVER (ORDER BY id) AS prev_val FROM timeline ORDER BY id",
				Want: [][]string{{"1", "10", "<nil>"}, {"2", "20", "10"}, {"3", "15", "20"}, {"4", "30", "15"}, {"5", "25", "30"}},
			},
			"lag_with_default": {
				SQL:  "SELECT id, value, LAG(value, 1, 0) OVER (ORDER BY id) AS prev_val FROM timeline ORDER BY id",
				Want: [][]string{{"1", "10", "0"}, {"2", "20", "10"}, {"3", "15", "20"}, {"4", "30", "15"}, {"5", "25", "30"}},
			},
			"lead": {
				SQL:  "SELECT id, value, LEAD(value) OVER (ORDER BY id) AS next_val FROM timeline ORDER BY id",
				Want: [][]string{{"1", "10", "20"}, {"2", "20", "15"}, {"3", "15", "30"}, {"4", "30", "25"}, {"5", "25", "<nil>"}},
			},
			"lag_offset_2": {
				SQL:  "SELECT id, LAG(value, 2) OVER (ORDER BY id) AS prev2 FROM timeline ORDER BY id",
				Want: [][]string{{"1", "<nil>"}, {"2", "<nil>"}, {"3", "10"}, {"4", "20"}, {"5", "15"}},
			},
		},
	})
}

func TestWindowFrames(t *testing.T) {
	run(t, testGroup{
		Name: "WINDOW_FRAMES",
		Setup: []string{
			"CREATE TABLE nums (id INTEGER PRIMARY KEY, val INTEGER)",
			"INSERT INTO nums VALUES (1, 10), (2, 20), (3, 30), (4, 40), (5, 50)",
		},
		Cases: map[string]testCase{
			"sum_unbounded_preceding": {
				SQL:  "SELECT id, SUM(val) OVER (ORDER BY id ROWS UNBOUNDED PRECEDING) AS running_sum FROM nums ORDER BY id",
				Want: [][]string{{"1", "10"}, {"2", "30"}, {"3", "60"}, {"4", "100"}, {"5", "150"}},
			},
			"sum_rows_between": {
				SQL:  "SELECT id, SUM(val) OVER (ORDER BY id ROWS BETWEEN 1 PRECEDING AND 1 FOLLOWING) AS window_sum FROM nums ORDER BY id",
				Want: [][]string{{"1", "30"}, {"2", "60"}, {"3", "90"}, {"4", "120"}, {"5", "90"}},
			},
			"avg_current_row": {
				SQL:  "SELECT id, AVG(val) OVER (ORDER BY id ROWS BETWEEN 2 PRECEDING AND CURRENT ROW) AS moving_avg FROM nums ORDER BY id",
				Want: [][]string{{"1", "10"}, {"2", "15"}, {"3", "20"}, {"4", "30"}, {"5", "40"}},
			},
			"count_window": {
				SQL:  "SELECT id, COUNT(*) OVER (ORDER BY id ROWS BETWEEN 1 PRECEDING AND 1 FOLLOWING) AS cnt FROM nums ORDER BY id",
				Want: [][]string{{"1", "2"}, {"2", "3"}, {"3", "3"}, {"4", "3"}, {"5", "2"}},
			},
		},
	})
}
