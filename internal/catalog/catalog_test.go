package catalog_test

import (
	"context"
	"sync"
	"testing"

	"github.com/yashikota/minipsql/internal/catalog"
	"github.com/yashikota/minipsql/internal/compat"
)

func newCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	ctx := context.Background()
	c, err := catalog.New(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close(ctx) })
	return c
}

func TestDefaultDatabaseExists(t *testing.T) {
	c := newCatalog(t)
	b, err := c.GetOrCreate("default")
	if err != nil {
		t.Fatalf("GetOrCreate(default) = %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestCreateAndGet(t *testing.T) {
	ctx := context.Background()
	c := newCatalog(t)
	if err := c.CreateDatabase(ctx, "mydb"); err != nil {
		t.Fatal(err)
	}
	b, err := c.GetOrCreate("mydb")
	if err != nil {
		t.Fatalf("GetOrCreate(mydb) = %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil backend")
	}
}

func TestCreateDuplicate(t *testing.T) {
	ctx := context.Background()
	c := newCatalog(t)
	if err := c.CreateDatabase(ctx, "dup"); err != nil {
		t.Fatal(err)
	}
	err := c.CreateDatabase(ctx, "dup")
	if err == nil {
		t.Fatal("expected error for duplicate, got nil")
	}
	var pg *compat.PGError
	if e, ok := err.(*compat.PGError); !ok || e.Code != "42P04" {
		_ = pg
		t.Fatalf("expected 42P04, got %v", err)
	}
}

func TestGetNonexistent(t *testing.T) {
	c := newCatalog(t)
	_, err := c.GetOrCreate("nosuchdb")
	if err == nil {
		t.Fatal("expected error for nonexistent DB, got nil")
	}
	if e, ok := err.(*compat.PGError); !ok || e.Code != "3D000" {
		t.Fatalf("expected 3D000, got %v", err)
	}
}

func TestDropExisting(t *testing.T) {
	ctx := context.Background()
	c := newCatalog(t)
	if err := c.CreateDatabase(ctx, "toDrop"); err != nil {
		t.Fatal(err)
	}
	if err := c.DropDatabase(ctx, "toDrop"); err != nil {
		t.Fatal(err)
	}
	_, err := c.GetOrCreate("todrop")
	if err == nil {
		t.Fatal("expected error after drop, got nil")
	}
}

func TestDropNonexistent(t *testing.T) {
	ctx := context.Background()
	c := newCatalog(t)
	err := c.DropDatabase(ctx, "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent drop, got nil")
	}
	if e, ok := err.(*compat.PGError); !ok || e.Code != "3D000" {
		t.Fatalf("expected 3D000, got %v", err)
	}
}

func TestDropDefault(t *testing.T) {
	ctx := context.Background()
	c := newCatalog(t)
	err := c.DropDatabase(ctx, "default")
	if err == nil {
		t.Fatal("expected error for dropping default, got nil")
	}
	if e, ok := err.(*compat.PGError); !ok || e.Code != "42501" {
		t.Fatalf("expected 42501, got %v", err)
	}
}

func TestConcurrentCreate(t *testing.T) {
	ctx := context.Background()
	c := newCatalog(t)
	const n = 20
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = c.CreateDatabase(ctx, "race")
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, e := range errs {
		if e == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 successful CreateDatabase, got %d", successes)
	}
}

func TestInitSQLRuns(t *testing.T) {
	ctx := context.Background()
	c, err := catalog.New(ctx, "CREATE TABLE IF NOT EXISTS _init_marker (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)

	b, err := c.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='_init_marker'", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("_init_marker table not found after initSQL; got %d rows", len(result.Rows))
	}
}
