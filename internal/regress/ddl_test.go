package regress_test

import "testing"

func TestCreateTable(t *testing.T) {
	run(t, testGroup{
		Name: "basic",
		Cases: map[string]testCase{
			"simple": {
				SQL:     "CREATE TABLE t1(id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
				WantTag: "CREATE TABLE",
			},
			"with_default": {
				SQL:     "CREATE TABLE t2(id INTEGER, status TEXT DEFAULT 'active')",
				WantTag: "CREATE TABLE",
			},
			"serial_type": {
				SQL:     "CREATE TABLE t3(id SERIAL PRIMARY KEY, name TEXT)",
				WantTag: "CREATE TABLE",
			},
			"multiple_types": {
				SQL:     "CREATE TABLE t4(a INTEGER, b TEXT, c REAL, d BLOB)",
				WantTag: "CREATE TABLE",
			},
		},
	})

	run(t, testGroup{
		Name:  "if_not_exists",
		Setup: []string{"CREATE TABLE t1(id INTEGER)"},
		Cases: map[string]testCase{
			"no_error_with_if_not_exists": {
				SQL:     "CREATE TABLE IF NOT EXISTS t1(id INTEGER)",
				WantTag: "CREATE TABLE",
			},
			"error_without_if_not_exists": {
				SQL:     "CREATE TABLE t1(id INTEGER)",
				WantErr: true,
			},
		},
	})
}

func TestDropTable(t *testing.T) {
	run(t, testGroup{
		Name:  "existing",
		Setup: []string{"CREATE TABLE t1(id INTEGER)"},
		Cases: map[string]testCase{
			"drop_existing": {
				SQL:     "DROP TABLE t1",
				WantTag: "DROP TABLE",
			},
		},
	})

	run(t, testGroup{
		Name: "nonexistent",
		Cases: map[string]testCase{
			"drop_nonexistent_error": {
				SQL:     "DROP TABLE nosuch",
				WantErr: true,
			},
		},
	})

	run(t, testGroup{
		Name: "if_exists",
		Cases: map[string]testCase{
			"drop_nonexistent_no_error": {
				SQL:     "DROP TABLE IF EXISTS nonexistent",
				WantTag: "DROP TABLE",
			},
		},
	})
}

func TestAlterTable(t *testing.T) {
	run(t, testGroup{
		Name:  "add_column",
		Setup: []string{"CREATE TABLE t1(id INTEGER, name TEXT)"},
		Cases: map[string]testCase{
			"add_column": {
				SQL:     "ALTER TABLE t1 ADD COLUMN age INTEGER",
				WantTag: "ALTER",
			},
		},
	})

	run(t, testGroup{
		Name:  "rename",
		Setup: []string{"CREATE TABLE t1(id INTEGER, name TEXT)"},
		Cases: map[string]testCase{
			"rename_table": {
				SQL:     "ALTER TABLE t1 RENAME TO t2",
				WantTag: "ALTER",
			},
		},
	})
}
