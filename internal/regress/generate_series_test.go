package regress_test

import "testing"

func TestGenerateSeries(t *testing.T) {
	run(t, testGroup{
		Name: "GENERATE_SERIES",
		Setup: []string{},
		Cases: map[string]testCase{
			"basic_series": {
				SQL:  "SELECT * FROM generate_series(1, 5)",
				Want: [][]string{{"1"}, {"2"}, {"3"}, {"4"}, {"5"}},
			},
			"series_with_step": {
				SQL:  "SELECT * FROM generate_series(0, 10, 2)",
				Want: [][]string{{"0"}, {"2"}, {"4"}, {"6"}, {"8"}, {"10"}},
			},
			"series_descending": {
				SQL:  "SELECT * FROM generate_series(5, 1, -1)",
				Want: [][]string{{"5"}, {"4"}, {"3"}, {"2"}, {"1"}},
			},
		},
	})
}

func TestGenerateSeriesJoin(t *testing.T) {
	run(t, testGroup{
		Name: "GENERATE_SERIES_JOIN",
		Setup: []string{
			"CREATE TABLE sparse (id INTEGER PRIMARY KEY, value TEXT)",
			"INSERT INTO sparse VALUES (1, 'one'), (3, 'three'), (5, 'five')",
		},
		Cases: map[string]testCase{
			"left_join_with_series": {
				SQL:  "SELECT g.val, s.value FROM generate_series(1, 5) AS g(val) LEFT JOIN sparse s ON s.id = g.val ORDER BY g.val",
				Want: [][]string{{"1", "one"}, {"2", "<nil>"}, {"3", "three"}, {"4", "<nil>"}, {"5", "five"}},
			},
		},
	})
}
