package catalog

import (
	"context"
	"testing"
)

func TestSaveAndLoadTemplate(t *testing.T) {
	ctx := context.Background()
	c, err := New(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)

	// Create test_setup database
	err = c.CreateDatabase(ctx, "regress_sql_test_setup_sql")
	if err != nil {
		t.Fatal(err)
	}

	b, err := c.GetOrCreate("regress_sql_test_setup_sql")
	if err != nil {
		t.Fatal(err)
	}

	// Create some tables and insert data
	_, err = b.Exec(ctx, "CREATE TABLE main.t1 (id integer, val text)", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(ctx, "INSERT INTO main.t1 VALUES (1, 'hello')", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Save to template
	err = c.saveToTemplate(ctx, b)
	if err != nil {
		t.Fatalf("saveToTemplate failed: %v", err)
	}

	// Create a new database and load from template
	err = c.CreateDatabase(ctx, "regress_sql_int2_sql")
	if err != nil {
		t.Fatal(err)
	}

	b2, err := c.GetOrCreate("regress_sql_int2_sql")
	if err != nil {
		t.Fatal(err)
	}

	// Query tables in b2
	res, err := b2.Query(ctx, "SELECT * FROM t1", nil)
	if err != nil {
		t.Fatalf("Query failed on b2: %v", err)
	}

	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}

	val := valStr(res.Rows[0][1])
	if val != "hello" {
		t.Fatalf("expected 'hello', got %q", val)
	}
}
