package regress_test

import "testing"

func TestInsert(t *testing.T) {
	run(t, testGroup{
		Name:  "single_row",
		Setup: []string{"CREATE TABLE t(id INTEGER, name TEXT)"},
		Cases: map[string]testCase{
			"values": {
				SQL:     "INSERT INTO t VALUES (1, 'alice')",
				WantTag: "INSERT 0 1",
			},
			"column_list": {
				SQL:     "INSERT INTO t(name, id) VALUES ('bob', 2)",
				WantTag: "INSERT 0 1",
			},
			"null_value": {
				SQL:     "INSERT INTO t VALUES (3, NULL)",
				WantTag: "INSERT 0 1",
			},
		},
	})

	run(t, testGroup{
		Name:  "multi_row",
		Setup: []string{"CREATE TABLE t(id INTEGER, name TEXT)"},
		Cases: map[string]testCase{
			"two_rows": {
				SQL:     "INSERT INTO t VALUES (1, 'a'), (2, 'b')",
				WantTag: "INSERT 0 2",
			},
			"three_rows": {
				SQL:     "INSERT INTO t VALUES (1, 'a'), (2, 'b'), (3, 'c')",
				WantTag: "INSERT 0 3",
			},
		},
	})

	run(t, testGroup{
		Name:  "returning",
		Setup: []string{"CREATE TABLE t(id INTEGER, name TEXT)"},
		Cases: map[string]testCase{
			"returning_all": {
				SQL:  "INSERT INTO t VALUES (1, 'alice') RETURNING *",
				Want: [][]string{{"1", "alice"}},
			},
			"returning_column": {
				SQL:  "INSERT INTO t VALUES (2, 'bob') RETURNING id",
				Want: [][]string{{"2"}},
			},
		},
	})
}

func TestUpdate(t *testing.T) {
	run(t, testGroup{
		Name: "basic",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT, age INTEGER)",
			"INSERT INTO t VALUES (1, 'alice', 30)",
			"INSERT INTO t VALUES (2, 'bob', 25)",
		},
		Cases: map[string]testCase{
			"set_one_column": {
				SQL:     "UPDATE t SET name = 'carol' WHERE id = 1",
				WantTag: "UPDATE 1",
			},
			"set_multiple_columns": {
				SQL:     "UPDATE t SET name = 'dave', age = 40 WHERE id = 2",
				WantTag: "UPDATE 1",
			},
			"update_all_rows": {
				SQL:     "UPDATE t SET age = 99",
				WantTag: "UPDATE 2",
			},
			"update_no_match": {
				SQL:     "UPDATE t SET name = 'x' WHERE id = 999",
				WantTag: "UPDATE 0",
			},
		},
	})
}

func TestDelete(t *testing.T) {
	run(t, testGroup{
		Name: "basic",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT)",
			"INSERT INTO t VALUES (1, 'alice')",
			"INSERT INTO t VALUES (2, 'bob')",
			"INSERT INTO t VALUES (3, 'carol')",
		},
		Cases: map[string]testCase{
			"delete_one": {
				SQL:     "DELETE FROM t WHERE id = 1",
				WantTag: "DELETE 1",
			},
			"delete_multiple": {
				SQL:     "DELETE FROM t WHERE id > 1",
				WantTag: "DELETE 2",
			},
			"delete_no_match": {
				SQL:     "DELETE FROM t WHERE id = 999",
				WantTag: "DELETE 0",
			},
		},
	})

	run(t, testGroup{
		Name: "delete_all",
		Setup: []string{
			"CREATE TABLE t(id INTEGER)",
			"INSERT INTO t VALUES (1)",
			"INSERT INTO t VALUES (2)",
		},
		Cases: map[string]testCase{
			"no_where": {
				SQL:     "DELETE FROM t",
				WantTag: "DELETE 2",
			},
		},
	})
}
