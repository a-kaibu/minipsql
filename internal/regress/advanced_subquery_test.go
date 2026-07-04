package regress_test

import "testing"

func TestCorrelatedSubquery(t *testing.T) {
	run(t, testGroup{
		Name: "CORRELATED_SUBQUERY",
		Setup: []string{
			"CREATE TABLE customers (id INTEGER PRIMARY KEY, name TEXT)",
			"INSERT INTO customers VALUES (1, 'alice'), (2, 'bob'), (3, 'carol')",
			"CREATE TABLE orders (id INTEGER PRIMARY KEY, customer_id INTEGER, amount INTEGER)",
			"INSERT INTO orders VALUES (1, 1, 100), (2, 1, 200), (3, 2, 150), (4, 1, 50), (5, 2, 300)",
		},
		Cases: map[string]testCase{
			"correlated_in_select": {
				SQL:  "SELECT c.name, (SELECT SUM(o.amount) FROM orders o WHERE o.customer_id = c.id) AS total FROM customers c ORDER BY c.name",
				Want: [][]string{{"alice", "350"}, {"bob", "450"}, {"carol", "<nil>"}},
			},
			"correlated_in_where": {
				SQL:  "SELECT c.name FROM customers c WHERE (SELECT COUNT(*) FROM orders o WHERE o.customer_id = c.id) > 1 ORDER BY c.name",
				Want: [][]string{{"alice"}, {"bob"}},
			},
			"correlated_exists": {
				SQL:  "SELECT c.name FROM customers c WHERE EXISTS (SELECT 1 FROM orders o WHERE o.customer_id = c.id AND o.amount > 200) ORDER BY c.name",
				Want: [][]string{{"bob"}},
			},
			"correlated_not_exists": {
				SQL:  "SELECT c.name FROM customers c WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.customer_id = c.id) ORDER BY c.name",
				Want: [][]string{{"carol"}},
			},
		},
	})
}

func TestSubqueryInHaving(t *testing.T) {
	run(t, testGroup{
		Name: "SUBQUERY_IN_HAVING",
		Setup: []string{
			"CREATE TABLE emp (id INTEGER PRIMARY KEY, dept TEXT, salary INTEGER)",
			"INSERT INTO emp VALUES (1, 'eng', 80000), (2, 'eng', 90000), (3, 'eng', 95000), (4, 'sales', 70000), (5, 'sales', 75000), (6, 'hr', 60000)",
		},
		Cases: map[string]testCase{
			"having_with_subquery": {
				SQL:  "SELECT dept, COUNT(*) AS cnt FROM emp GROUP BY dept HAVING COUNT(*) > (SELECT COUNT(*) FROM emp WHERE dept = 'hr') ORDER BY dept",
				Want: [][]string{{"eng", "3"}, {"sales", "2"}},
			},
			"having_avg_subquery": {
				SQL:  "SELECT dept, AVG(salary) AS avg_sal FROM emp GROUP BY dept HAVING AVG(salary) > (SELECT AVG(salary) FROM emp) ORDER BY dept",
				Want: [][]string{{"eng", "88333.33333333333"}},
			},
		},
	})
}

func TestSubqueryInFrom(t *testing.T) {
	run(t, testGroup{
		Name: "SUBQUERY_IN_FROM",
		Setup: []string{
			"CREATE TABLE products (id INTEGER PRIMARY KEY, category TEXT, price INTEGER)",
			"INSERT INTO products VALUES (1, 'A', 10), (2, 'A', 20), (3, 'B', 30), (4, 'B', 40), (5, 'C', 50)",
		},
		Cases: map[string]testCase{
			"derived_table_with_aggregate": {
				SQL:  "SELECT sub.category, sub.total FROM (SELECT category, SUM(price) AS total FROM products GROUP BY category) sub WHERE sub.total > 25 ORDER BY sub.category",
				Want: [][]string{{"A", "30"}, {"B", "70"}, {"C", "50"}},
			},
			"nested_derived_tables": {
				SQL:  "SELECT category FROM (SELECT category, cnt FROM (SELECT category, COUNT(*) AS cnt FROM products GROUP BY category) inner_sub WHERE cnt > 1) outer_sub ORDER BY category",
				Want: [][]string{{"A"}, {"B"}},
			},
		},
	})
}
