package regress_test

import "testing"

func TestOrderByExpression(t *testing.T) {
	run(t, testGroup{
		Name: "ORDER_BY_EXPRESSION",
		Setup: []string{
			"CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, value INTEGER)",
			"INSERT INTO items VALUES (1, 'short', 30), (2, 'very long name', 10), (3, 'medium', 20), (4, 'ab', 40)",
		},
		Cases: map[string]testCase{
			"order_by_length": {
				SQL:  "SELECT name FROM items ORDER BY LENGTH(name)",
				Want: [][]string{{"ab"}, {"short"}, {"medium"}, {"very long name"}},
			},
			"order_by_arithmetic": {
				SQL:  "SELECT id, value FROM items ORDER BY value * -1",
				Want: [][]string{{"4", "40"}, {"1", "30"}, {"3", "20"}, {"2", "10"}},
			},
			"order_by_case": {
				SQL:  "SELECT id, name FROM items ORDER BY CASE WHEN LENGTH(name) < 4 THEN 0 ELSE 1 END, id",
				Want: [][]string{{"4", "ab"}, {"1", "short"}, {"2", "very long name"}, {"3", "medium"}},
			},
			"order_by_modulo": {
				SQL:  "SELECT id, value FROM items ORDER BY value % 20, id",
				Want: [][]string{{"3", "20"}, {"4", "40"}, {"1", "30"}, {"2", "10"}},
			},
		},
	})
}

func TestOrderByNulls(t *testing.T) {
	run(t, testGroup{
		Name: "ORDER_BY_NULLS",
		Setup: []string{
			"CREATE TABLE nullable (id INTEGER PRIMARY KEY, value INTEGER)",
			"INSERT INTO nullable VALUES (1, 10), (2, NULL), (3, 30), (4, NULL), (5, 20)",
		},
		Cases: map[string]testCase{
			"nulls_first": {
				SQL:  "SELECT id, value FROM nullable ORDER BY value NULLS FIRST, id",
				Skip: "not yet implemented: NULLS FIRST/LAST",
				Want: [][]string{{"2", "<nil>"}, {"4", "<nil>"}, {"1", "10"}, {"5", "20"}, {"3", "30"}},
			},
			"nulls_last": {
				SQL:  "SELECT id, value FROM nullable ORDER BY value NULLS LAST, id",
				Skip: "not yet implemented: NULLS FIRST/LAST",
				Want: [][]string{{"1", "10"}, {"5", "20"}, {"3", "30"}, {"2", "<nil>"}, {"4", "<nil>"}},
			},
			"desc_nulls_first": {
				SQL:  "SELECT id, value FROM nullable ORDER BY value DESC NULLS FIRST, id",
				Skip: "not yet implemented: NULLS FIRST/LAST",
				Want: [][]string{{"2", "<nil>"}, {"4", "<nil>"}, {"3", "30"}, {"5", "20"}, {"1", "10"}},
			},
		},
	})
}
