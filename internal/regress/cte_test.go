package regress_test

import "testing"

func TestCTEBasic(t *testing.T) {
	run(t, testGroup{
		Name: "CTE_BASIC",
		Setup: []string{
			"CREATE TABLE employees (id INTEGER PRIMARY KEY, name TEXT, dept TEXT, salary INTEGER)",
			"INSERT INTO employees VALUES (1, 'alice', 'eng', 80000), (2, 'bob', 'eng', 90000), (3, 'carol', 'sales', 70000), (4, 'dave', 'sales', 75000), (5, 'eve', 'eng', 95000)",
		},
		Cases: map[string]testCase{
			"simple_cte": {
				SQL:  "WITH eng AS (SELECT name, salary FROM employees WHERE dept = 'eng') SELECT name FROM eng ORDER BY name",
				Want: [][]string{{"alice"}, {"bob"}, {"eve"}},
			},
			"cte_with_aggregate": {
				SQL:  "WITH dept_avg AS (SELECT dept, AVG(salary) AS avg_sal FROM employees GROUP BY dept) SELECT dept, avg_sal FROM dept_avg ORDER BY dept",
				Want: [][]string{{"eng", "88333.33333333333"}, {"sales", "72500"}},
			},
			"multiple_ctes": {
				SQL:  "WITH eng AS (SELECT name, salary FROM employees WHERE dept = 'eng'), high_earners AS (SELECT name FROM eng WHERE salary > 85000) SELECT name FROM high_earners ORDER BY name",
				Want: [][]string{{"bob"}, {"eve"}},
			},
			"cte_used_twice": {
				SQL:  "WITH base AS (SELECT dept, COUNT(*) AS cnt FROM employees GROUP BY dept) SELECT b1.dept FROM base b1 WHERE b1.cnt = (SELECT MAX(cnt) FROM base)",
				Want: [][]string{{"eng"}},
			},
		},
	})
}

func TestCTERecursive(t *testing.T) {
	run(t, testGroup{
		Name: "CTE_RECURSIVE",
		Setup: []string{
			"CREATE TABLE tree (id INTEGER PRIMARY KEY, parent_id INTEGER, name TEXT)",
			"INSERT INTO tree VALUES (1, NULL, 'root'), (2, 1, 'child1'), (3, 1, 'child2'), (4, 2, 'grandchild1'), (5, 2, 'grandchild2'), (6, 3, 'grandchild3')",
		},
		Cases: map[string]testCase{
			"recursive_series": {
				SQL:  "WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM cnt WHERE x < 5) SELECT x FROM cnt",
				Want: [][]string{{"1"}, {"2"}, {"3"}, {"4"}, {"5"}},
			},
			"recursive_tree_traversal": {
				SQL:  "WITH RECURSIVE subtree AS (SELECT id, name, 0 AS depth FROM tree WHERE id = 1 UNION ALL SELECT t.id, t.name, s.depth + 1 FROM tree t JOIN subtree s ON t.parent_id = s.id) SELECT name, depth FROM subtree ORDER BY id",
				Want: [][]string{{"root", "0"}, {"child1", "1"}, {"child2", "1"}, {"grandchild1", "2"}, {"grandchild2", "2"}, {"grandchild3", "2"}},
			},
			"recursive_path": {
				SQL:  "WITH RECURSIVE path AS (SELECT id, name, name AS full_path FROM tree WHERE parent_id IS NULL UNION ALL SELECT t.id, t.name, p.full_path || '/' || t.name FROM tree t JOIN path p ON t.parent_id = p.id) SELECT full_path FROM path WHERE id = 4",
				Want: [][]string{{"root/child1/grandchild1"}},
			},
		},
	})
}
