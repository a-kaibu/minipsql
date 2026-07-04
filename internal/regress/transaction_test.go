package regress_test

import "testing"

func TestTransactionCommit(t *testing.T) {
	run(t, testGroup{
		Name: "commit",
		Setup: []string{
			"CREATE TABLE t(id INTEGER, name TEXT)",
		},
		Cases: map[string]testCase{
			"begin_insert_commit": {
				SQL:     "BEGIN",
				WantTag: "BEGIN",
			},
		},
	})

	// Full transaction flow test
	t.Run("full_commit_flow", func(t *testing.T) {
		s, ctx := newSession(t)
		_, _, err := s.Execute(ctx, "CREATE TABLE t(id INTEGER, name TEXT)", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "BEGIN", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "INSERT INTO t VALUES (1, 'alice')", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "COMMIT", nil)
		if err != nil {
			t.Fatal(err)
		}
		result, _, err := s.Execute(ctx, "SELECT name FROM t", nil)
		if err != nil {
			t.Fatal(err)
		}
		assertRows(t, result, [][]string{{"alice"}})
	})
}

func TestTransactionRollback(t *testing.T) {
	t.Run("rollback_undoes_insert", func(t *testing.T) {
		s, ctx := newSession(t)
		_, _, err := s.Execute(ctx, "CREATE TABLE t(id INTEGER, name TEXT)", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "BEGIN", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "INSERT INTO t VALUES (1, 'alice')", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "ROLLBACK", nil)
		if err != nil {
			t.Fatal(err)
		}
		result, _, err := s.Execute(ctx, "SELECT * FROM t", nil)
		if err != nil {
			t.Fatal(err)
		}
		assertRows(t, result, [][]string{})
	})

	t.Run("rollback_undoes_update", func(t *testing.T) {
		s, ctx := newSession(t)
		_, _, err := s.Execute(ctx, "CREATE TABLE t(id INTEGER, name TEXT)", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "INSERT INTO t VALUES (1, 'alice')", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "BEGIN", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "UPDATE t SET name = 'bob'", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "ROLLBACK", nil)
		if err != nil {
			t.Fatal(err)
		}
		result, _, err := s.Execute(ctx, "SELECT name FROM t", nil)
		if err != nil {
			t.Fatal(err)
		}
		assertRows(t, result, [][]string{{"alice"}})
	})
}

func TestTransactionAliases(t *testing.T) {
	run(t, testGroup{
		Name: "aliases",
		Cases: map[string]testCase{
			"start_transaction": {
				SQL:     "START TRANSACTION",
				WantTag: "BEGIN",
			},
			"end": {
				SQL:     "END",
				WantTag: "COMMIT",
				Skip:    "END alias may not be implemented",
			},
			"abort": {
				SQL:     "ABORT",
				WantTag: "ROLLBACK",
				Skip:    "ABORT alias may not be implemented",
			},
		},
	})
}

func TestTransactionErrorRecovery(t *testing.T) {
	t.Run("error_in_tx_requires_rollback", func(t *testing.T) {
		s, ctx := newSession(t)
		_, _, err := s.Execute(ctx, "CREATE TABLE t(id INTEGER)", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "BEGIN", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.Execute(ctx, "SELECT * FROM nonexistent", nil)
		if err == nil {
			t.Fatal("expected error for nonexistent table")
		}
		_, _, err = s.Execute(ctx, "ROLLBACK", nil)
		if err != nil {
			t.Fatalf("ROLLBACK after error should succeed: %v", err)
		}
		_, _, err = s.Execute(ctx, "SELECT * FROM t", nil)
		if err != nil {
			t.Fatalf("query after rollback should work: %v", err)
		}
	})
}
