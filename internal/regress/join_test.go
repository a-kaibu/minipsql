package regress_test

import "testing"

func TestJoinInner(t *testing.T) {
	run(t, testGroup{
		Name: "inner_join",
		Setup: []string{
			"CREATE TABLE depts(id INTEGER, name TEXT)",
			"INSERT INTO depts VALUES (1, 'eng'), (2, 'sales'), (3, 'hr')",
			"CREATE TABLE emps(id INTEGER, name TEXT, dept_id INTEGER)",
			"INSERT INTO emps VALUES (1, 'alice', 1), (2, 'bob', 2), (3, 'carol', NULL)",
		},
		Cases: map[string]testCase{
			"basic": {
				SQL:  "SELECT e.name, d.name FROM emps e INNER JOIN depts d ON e.dept_id = d.id ORDER BY e.name",
				Want: [][]string{{"alice", "eng"}, {"bob", "sales"}},
			},
			"with_where": {
				SQL:  "SELECT e.name, d.name FROM emps e INNER JOIN depts d ON e.dept_id = d.id WHERE d.name = 'eng'",
				Want: [][]string{{"alice", "eng"}},
			},
			"implicit_join": {
				SQL:  "SELECT e.name, d.name FROM emps e, depts d WHERE e.dept_id = d.id ORDER BY e.name",
				Want: [][]string{{"alice", "eng"}, {"bob", "sales"}},
			},
		},
	})
}

func TestJoinLeft(t *testing.T) {
	run(t, testGroup{
		Name: "left_join",
		Setup: []string{
			"CREATE TABLE depts(id INTEGER, name TEXT)",
			"INSERT INTO depts VALUES (1, 'eng'), (2, 'sales'), (3, 'hr')",
			"CREATE TABLE emps(id INTEGER, name TEXT, dept_id INTEGER)",
			"INSERT INTO emps VALUES (1, 'alice', 1), (2, 'bob', 2), (3, 'carol', NULL)",
		},
		Cases: map[string]testCase{
			"basic": {
				SQL:  "SELECT e.name, d.name FROM emps e LEFT JOIN depts d ON e.dept_id = d.id ORDER BY e.name",
				Want: [][]string{{"alice", "eng"}, {"bob", "sales"}, {"carol", "<nil>"}},
			},
			"left_outer": {
				SQL:  "SELECT e.name, d.name FROM emps e LEFT OUTER JOIN depts d ON e.dept_id = d.id ORDER BY e.name",
				Want: [][]string{{"alice", "eng"}, {"bob", "sales"}, {"carol", "<nil>"}},
			},
		},
	})
}

func TestJoinRight(t *testing.T) {
	run(t, testGroup{
		Name: "right_join",
		Setup: []string{
			"CREATE TABLE depts(id INTEGER, name TEXT)",
			"INSERT INTO depts VALUES (1, 'eng'), (2, 'sales'), (3, 'hr')",
			"CREATE TABLE emps(id INTEGER, name TEXT, dept_id INTEGER)",
			"INSERT INTO emps VALUES (1, 'alice', 1), (2, 'bob', 2)",
		},
		Cases: map[string]testCase{
			"basic": {
				SQL:  "SELECT e.name, d.name FROM emps e RIGHT JOIN depts d ON e.dept_id = d.id ORDER BY d.name",
				Want: [][]string{{"alice", "eng"}, {"<nil>", "hr"}, {"bob", "sales"}},
			},
		},
	})
}

func TestJoinCross(t *testing.T) {
	run(t, testGroup{
		Name: "cross_join",
		Setup: []string{
			"CREATE TABLE a(x INTEGER)",
			"INSERT INTO a VALUES (1), (2)",
			"CREATE TABLE b(y TEXT)",
			"INSERT INTO b VALUES ('a'), ('b')",
		},
		Cases: map[string]testCase{
			"basic": {
				SQL:  "SELECT x, y FROM a CROSS JOIN b ORDER BY x, y",
				Want: [][]string{{"1", "a"}, {"1", "b"}, {"2", "a"}, {"2", "b"}},
			},
		},
	})
}

func TestJoinSelf(t *testing.T) {
	run(t, testGroup{
		Name: "self_join",
		Setup: []string{
			"CREATE TABLE emps(id INTEGER, name TEXT, manager_id INTEGER)",
			"INSERT INTO emps VALUES (1, 'alice', NULL), (2, 'bob', 1), (3, 'carol', 1)",
		},
		Cases: map[string]testCase{
			"basic": {
				SQL:  "SELECT e.name, m.name FROM emps e INNER JOIN emps m ON e.manager_id = m.id ORDER BY e.name",
				Want: [][]string{{"bob", "alice"}, {"carol", "alice"}},
			},
		},
	})
}
