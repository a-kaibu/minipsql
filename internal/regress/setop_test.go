package regress_test

import "testing"

func TestSetOpUnion(t *testing.T) {
	run(t, testGroup{
		Name: "UNION",
		Setup: []string{
			"CREATE TABLE t1 (id INTEGER, name TEXT)",
			"INSERT INTO t1 VALUES (1, 'alice'), (2, 'bob'), (3, 'carol')",
			"CREATE TABLE t2 (id INTEGER, name TEXT)",
			"INSERT INTO t2 VALUES (2, 'bob'), (3, 'carol'), (4, 'dave')",
		},
		Cases: map[string]testCase{
			"union_dedup": {
				SQL:  "SELECT name FROM t1 UNION SELECT name FROM t2 ORDER BY name",
				Want: [][]string{{"alice"}, {"bob"}, {"carol"}, {"dave"}},
			},
			"union_all": {
				SQL:  "SELECT name FROM t1 UNION ALL SELECT name FROM t2 ORDER BY name",
				Want: [][]string{{"alice"}, {"bob"}, {"bob"}, {"carol"}, {"carol"}, {"dave"}},
			},
			"union_with_where": {
				SQL:  "SELECT name FROM t1 WHERE id > 1 UNION SELECT name FROM t2 WHERE id < 4 ORDER BY name",
				Want: [][]string{{"bob"}, {"carol"}},
			},
		},
	})
}

func TestSetOpIntersectExcept(t *testing.T) {
	run(t, testGroup{
		Name: "INTERSECT_EXCEPT",
		Setup: []string{
			"CREATE TABLE s1 (v INTEGER)",
			"INSERT INTO s1 VALUES (1), (2), (3), (4), (5)",
			"CREATE TABLE s2 (v INTEGER)",
			"INSERT INTO s2 VALUES (3), (4), (5), (6), (7)",
		},
		Cases: map[string]testCase{
			"intersect": {
				SQL:  "SELECT v FROM s1 INTERSECT SELECT v FROM s2 ORDER BY v",
				Want: [][]string{{"3"}, {"4"}, {"5"}},
			},
			"except": {
				SQL:  "SELECT v FROM s1 EXCEPT SELECT v FROM s2 ORDER BY v",
				Want: [][]string{{"1"}, {"2"}},
			},
			"except_reversed": {
				SQL:  "SELECT v FROM s2 EXCEPT SELECT v FROM s1 ORDER BY v",
				Want: [][]string{{"6"}, {"7"}},
			},
		},
	})
}

func TestSetOpChained(t *testing.T) {
	run(t, testGroup{
		Name: "CHAINED_SET_OPS",
		Setup: []string{
			"CREATE TABLE a (v INTEGER)",
			"INSERT INTO a VALUES (1), (2), (3)",
			"CREATE TABLE b (v INTEGER)",
			"INSERT INTO b VALUES (2), (3), (4)",
			"CREATE TABLE c (v INTEGER)",
			"INSERT INTO c VALUES (3), (4), (5)",
		},
		Cases: map[string]testCase{
			"union_chain": {
				SQL:  "SELECT v FROM a UNION SELECT v FROM b UNION SELECT v FROM c ORDER BY v",
				Want: [][]string{{"1"}, {"2"}, {"3"}, {"4"}, {"5"}},
			},
			"union_then_intersect": {
				SQL:  "SELECT v FROM a INTERSECT SELECT v FROM b INTERSECT SELECT v FROM c ORDER BY v",
				Want: [][]string{{"3"}},
			},
			"mixed_ops": {
				SQL:  "SELECT v FROM a UNION SELECT v FROM b EXCEPT SELECT v FROM c ORDER BY v",
				Want: [][]string{{"1"}, {"2"}},
			},
		},
	})
}
