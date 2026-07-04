package regress_test

import "testing"

func TestTransactionCommit(t *testing.T) {
	run(t, testGroup{
		Name:  "commit",
		Setup: []string{"CREATE TABLE t(id INTEGER, name TEXT)"},
		Cases: map[string]testCase{
			"begin": {
				SQL:     "BEGIN",
				WantTag: "BEGIN",
			},
		},
	})

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
	t.Run("start_transaction", func(t *testing.T) {
		s, ctx := newSession(t)
		_, tag, err := s.Execute(ctx, "START TRANSACTION", nil)
		if err != nil {
			t.Fatal(err)
		}
		if tag.Tag != "BEGIN" {
			t.Fatalf("tag = %q, want BEGIN", tag.Tag)
		}
		s.Execute(ctx, "ROLLBACK", nil)
	})

	t.Run("end_commits", func(t *testing.T) {
		s, ctx := newSession(t)
		s.Execute(ctx, "BEGIN", nil)
		_, tag, err := s.Execute(ctx, "END", nil)
		if err != nil {
			t.Fatal(err)
		}
		if tag.Tag != "COMMIT" {
			t.Fatalf("tag = %q, want COMMIT", tag.Tag)
		}
	})

	t.Run("abort_rolls_back", func(t *testing.T) {
		s, ctx := newSession(t)
		s.Execute(ctx, "BEGIN", nil)
		_, tag, err := s.Execute(ctx, "ABORT", nil)
		if err != nil {
			t.Fatal(err)
		}
		if tag.Tag != "ROLLBACK" {
			t.Fatalf("tag = %q, want ROLLBACK", tag.Tag)
		}
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
