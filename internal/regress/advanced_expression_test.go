package regress_test

import "testing"

func TestNestedCase(t *testing.T) {
	run(t, testGroup{
		Name: "NESTED_CASE",
		Setup: []string{
			"CREATE TABLE items (id INTEGER PRIMARY KEY, value INTEGER, status TEXT)",
			"INSERT INTO items VALUES (1, 5, 'active'), (2, 15, 'active'), (3, -3, 'inactive'), (4, 25, 'active'), (5, 0, 'inactive')",
		},
		Cases: map[string]testCase{
			"nested_case_in_select": {
				SQL: "SELECT id, CASE WHEN value > 0 THEN CASE WHEN value > 10 THEN 'high' ELSE 'low' END ELSE 'negative' END AS category FROM items ORDER BY id",
				Want: [][]string{{"1", "low"}, {"2", "high"}, {"3", "negative"}, {"4", "high"}, {"5", "negative"}},
			},
			"case_in_where": {
				SQL:  "SELECT id FROM items WHERE CASE WHEN status = 'active' THEN value > 10 ELSE value >= 0 END ORDER BY id",
				Want: [][]string{{"2"}, {"4"}, {"5"}},
			},
			"case_in_order_by": {
				SQL:  "SELECT id, status FROM items ORDER BY CASE WHEN status = 'active' THEN 0 ELSE 1 END, id",
				Want: [][]string{{"1", "active"}, {"2", "active"}, {"4", "active"}, {"3", "inactive"}, {"5", "inactive"}},
			},
			"case_in_aggregate": {
				SQL:  "SELECT SUM(CASE WHEN status = 'active' THEN value ELSE 0 END) AS active_sum FROM items",
				Want: [][]string{{"45"}},
			},
		},
	})
}

func TestTypeCastingAdvanced(t *testing.T) {
	run(t, testGroup{
		Name: "TYPE_CASTING_ADVANCED",
		Setup: []string{
			"CREATE TABLE mixed (id INTEGER PRIMARY KEY, num_text TEXT, val REAL)",
			"INSERT INTO mixed VALUES (1, '42', 3.14), (2, '100', 2.71), (3, '7', 1.41)",
		},
		Cases: map[string]testCase{
			"cast_text_to_int": {
				SQL:  "SELECT CAST(num_text AS INTEGER) + 1 AS result FROM mixed ORDER BY id",
				Want: [][]string{{"43"}, {"101"}, {"8"}},
			},
			"cast_int_to_text": {
				SQL:  "SELECT CAST(id AS TEXT) || '-item' AS label FROM mixed ORDER BY id",
				Want: [][]string{{"1-item"}, {"2-item"}, {"3-item"}},
			},
			"cast_real_to_int": {
				SQL:  "SELECT id, CAST(val AS INTEGER) AS truncated FROM mixed ORDER BY id",
				Want: [][]string{{"1", "3"}, {"2", "2"}, {"3", "1"}},
			},
			"pg_cast_syntax": {
				SQL:  "SELECT 42::TEXT",
				Want: [][]string{{"42"}},
			},
			"pg_cast_in_expression": {
				SQL:  "SELECT (id::TEXT) || '-' || num_text AS combined FROM mixed ORDER BY id",
				Want: [][]string{{"1-42"}, {"2-100"}, {"3-7"}},
			},
		},
	})
}
