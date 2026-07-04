package regress_test

import "testing"

func TestSubqueryScalar(t *testing.T) {
	run(t, testGroup{
		Name: "scalar",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, value INTEGER)",
			"INSERT INTO t VALUES (1, 10), (2, 20), (3, 30)",
		},
		Cases: map[string]testCase{
			"in_select": {
				SQL:  "SELECT (SELECT MAX(value) FROM t)",
				Want: [][]string{{"30"}},
			},
			"in_where": {
				SQL:  "SELECT id FROM t WHERE value = (SELECT MAX(value) FROM t)",
				Want: [][]string{{"3"}},
			},
		},
	})
}

func TestSubqueryIn(t *testing.T) {
	run(t, testGroup{
		Name: "in_subquery",
		Setup: []string{
			"CREATE TABLE depts(id INTEGER, name TEXT)",
			"INSERT INTO depts VALUES (1, 'eng'), (2, 'sales')",
			"CREATE TABLE emps(id INTEGER, name TEXT, dept_id INTEGER)",
			"INSERT INTO emps VALUES (1, 'alice', 1), (2, 'bob', 2), (3, 'carol', 3)",
		},
		Cases: map[string]testCase{
			"in": {
				SQL:  "SELECT name FROM emps WHERE dept_id IN (SELECT id FROM depts) ORDER BY name",
				Want: [][]string{{"alice"}, {"bob"}},
			},
			"not_in": {
				SQL:  "SELECT name FROM emps WHERE dept_id NOT IN (SELECT id FROM depts) ORDER BY name",
				Want: [][]string{{"carol"}},
			},
		},
	})
}

func TestSubqueryExists(t *testing.T) {
	run(t, testGroup{
		Name: "exists",
		Setup: []string{
			"CREATE TABLE orders(id INTEGER, customer_id INTEGER)",
			"INSERT INTO orders VALUES (1, 1), (2, 1)",
			"CREATE TABLE customers(id INTEGER, name TEXT)",
			"INSERT INTO customers VALUES (1, 'alice'), (2, 'bob')",
		},
		Cases: map[string]testCase{
			"exists_true": {
				SQL:  "SELECT name FROM customers c WHERE EXISTS (SELECT 1 FROM orders o WHERE o.customer_id = c.id) ORDER BY name",
				Want: [][]string{{"alice"}},
			},
			"not_exists": {
				SQL:  "SELECT name FROM customers c WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.customer_id = c.id) ORDER BY name",
				Want: [][]string{{"bob"}},
			},
		},
	})
}

func TestSubqueryDerived(t *testing.T) {
	run(t, testGroup{
		Name: "derived_table",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, value INTEGER)",
			"INSERT INTO t VALUES (1, 10), (2, 20), (3, 30)",
		},
		Cases: map[string]testCase{
			"from_subquery": {
				SQL:  "SELECT s.total FROM (SELECT SUM(value) AS total FROM t) s",
				Want: [][]string{{"60"}},
			},
		},
	})
}
