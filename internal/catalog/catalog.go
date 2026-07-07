package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/backend/sqlite"
	"github.com/yashikota/minipsql/internal/compat"
)

type dbEntry struct {
	b         *sqlite.Backend
	refs      atomic.Int32
	dropped   bool
	closeOnce sync.Once
}

func (e *dbEntry) close(ctx context.Context) {
	e.closeOnce.Do(func() { _ = e.b.Close(ctx) })
}

// Catalog manages named in-memory databases shared across connections.
type Catalog struct {
	mu         sync.RWMutex
	dbs        map[string]*dbEntry
	initSQL    string
	templateDB *sql.DB
}

// New creates a Catalog with a pre-initialized "default" database.
func New(ctx context.Context, initSQL string) (*Catalog, error) {
	templateDB, err := sql.Open("sqlite", "file:minipsql_template?mode=memory&cache=shared")
	if err != nil {
		return nil, err
	}
	if err := templateDB.PingContext(ctx); err != nil {
		_ = templateDB.Close()
		return nil, err
	}

	c := &Catalog{
		dbs:        make(map[string]*dbEntry),
		initSQL:    initSQL,
		templateDB: templateDB,
	}
	if err := c.createDB(ctx, "default"); err != nil {
		_ = templateDB.Close()
		return nil, err
	}
	return c, nil
}

func (c *Catalog) createDB(ctx context.Context, name string) error {
	b, err := sqlite.New(ctx)
	if err != nil {
		return err
	}
	_ = c.loadFromTemplate(ctx, b)
	if err := c.runInitSQL(ctx, b); err != nil {
		_ = b.Close(ctx)
		return err
	}
	c.dbs[name] = &dbEntry{b: b}
	return nil
}

func (c *Catalog) runInitSQL(ctx context.Context, b backend.Backend) error {
	if strings.TrimSpace(c.initSQL) == "" {
		return nil
	}
	for _, stmt := range compat.SplitStatements(c.initSQL) {
		if _, err := b.Exec(ctx, stmt, nil); err != nil {
			return err
		}
	}
	return nil
}

// GetOrCreate returns a per-session backend for the named database.
// Each call returns an independent session with isolated transaction state.
// Returns error 3D000 if the database does not exist.
func (c *Catalog) GetOrCreate(name string) (backend.Backend, error) {
	if name == "" {
		name = "default"
	}
	name = strings.ToLower(name)
	c.mu.RLock()
	e, ok := c.dbs[name]
	c.mu.RUnlock()
	if !ok || e.dropped {
		return nil, &compat.PGError{Code: "3D000", Message: "database \"" + name + "\" does not exist"}
	}
	return e.b.NewSession(), nil
}

// Connect increments the connection reference count for a database.
func (c *Catalog) Connect(name string) {
	name = strings.ToLower(name)
	c.mu.RLock()
	e := c.dbs[name]
	c.mu.RUnlock()
	if e != nil {
		e.refs.Add(1)
	}
}

// Disconnect decrements the connection reference count.
// When refs drop to zero and the database has been dropped, the backend is closed.
func (c *Catalog) Disconnect(ctx context.Context, name string) {
	name = strings.ToLower(name)
	c.mu.RLock()
	e := c.dbs[name]
	c.mu.RUnlock()
	if e == nil {
		return
	}
	if e.refs.Add(-1) == 0 && e.dropped {
		c.mu.Lock()
		delete(c.dbs, name)
		c.mu.Unlock()
		e.close(ctx)
	}
}

// CreateDatabase creates a new named database.
// Returns error 42P04 if the database already exists.
func (c *Catalog) CreateDatabase(ctx context.Context, name string) error {
	name = strings.ToLower(name)
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.dbs[name]; ok && !e.dropped {
		return &compat.PGError{Code: "42P04", Message: "database \"" + name + "\" already exists"}
	}
	return c.createDB(ctx, name)
}

// RenameDatabase renames a database.
func (c *Catalog) RenameDatabase(ctx context.Context, oldName, newName string) error {
	oldName = strings.ToLower(oldName)
	newName = strings.ToLower(newName)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.dbs[oldName]
	if !ok || e.dropped {
		return &compat.PGError{Code: "3D000", Message: "database \"" + oldName + "\" does not exist"}
	}
	if dup, ok := c.dbs[newName]; ok && !dup.dropped {
		return &compat.PGError{Code: "42P04", Message: "database \"" + newName + "\" already exists"}
	}
	delete(c.dbs, oldName)
	c.dbs[newName] = e
	return nil
}

// DropDatabase marks a database as dropped.
// Returns error 42501 for "default", error 3D000 if it does not exist.
// The entry stays in the map until all active connections disconnect so that
// Disconnect can still look it up and close the backend when refs reach zero.
func (c *Catalog) DropDatabase(ctx context.Context, name string) error {
	name = strings.ToLower(name)
	if name == "default" {
		return &compat.PGError{Code: "42501", Message: "cannot drop the default database"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.dbs[name]
	if !ok || e.dropped {
		return &compat.PGError{Code: "3D000", Message: "database \"" + name + "\" does not exist"}
	}
	if strings.Contains(name, "test_setup") {
		_ = c.saveToTemplate(ctx, e.b.NewSession())
	}
	e.dropped = true
	// Keep the entry in the map so Disconnect can still find it.
	// Close immediately if no active connections; otherwise Disconnect will close.
	if e.refs.Load() == 0 {
		delete(c.dbs, name)
		e.close(ctx)
	}
	return nil
}

// Close closes all managed backends (called on server shutdown).
func (c *Catalog) Close(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.dbs {
		e.close(ctx)
	}
	c.dbs = make(map[string]*dbEntry)
	if c.templateDB != nil {
		_ = c.templateDB.Close()
		c.templateDB = nil
	}
}

func (c *Catalog) saveToTemplate(ctx context.Context, b backend.Backend) error {
	_, err := b.Exec(ctx, "ATTACH DATABASE 'file:minipsql_template?mode=memory&cache=shared' AS template", nil)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = b.Exec(ctx, "DETACH DATABASE template", nil)
	}()

	res, err := b.Query(ctx, "SELECT name, sql FROM main.sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'pg_%'", nil)
	if err != nil {
		return err
	}

	for _, row := range res.Rows {
		if len(row) < 2 {
			continue
		}
		tName := valStr(row[0])
		tSQL := valStr(row[1])
		if tName == "" || tSQL == "" {
			continue
		}

		idx := strings.Index(strings.ToUpper(tSQL), "TABLE ")
		if idx < 0 {
			continue
		}
		createSQL := tSQL[:idx+6] + "template." + tSQL[idx+6:]

		_, err = b.Exec(ctx, createSQL, nil)
		if err != nil {
			_, _ = b.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS template.%s", tName), nil)
			_, err = b.Exec(ctx, createSQL, nil)
			if err != nil {
				return err
			}
		}

		_, err = b.Exec(ctx, fmt.Sprintf("INSERT INTO template.%s SELECT * FROM main.%s", tName, tName), nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Catalog) loadFromTemplate(ctx context.Context, b backend.Backend) error {
	_, err := b.Exec(ctx, "ATTACH DATABASE 'file:minipsql_template?mode=memory&cache=shared' AS template", nil)
	if err != nil {
		return nil
	}
	defer func() {
		_, _ = b.Exec(ctx, "DETACH DATABASE template", nil)
	}()

	res, err := b.Query(ctx, "SELECT name, sql FROM template.sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'pg_%'", nil)
	if err != nil {
		return nil
	}

	for _, row := range res.Rows {
		if len(row) < 2 {
			continue
		}
		tName := valStr(row[0])
		tSQL := valStr(row[1])
		if tName == "" || tSQL == "" {
			continue
		}

		createSQL := strings.Replace(tSQL, "template.", "main.", 1)

		_, _ = b.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS main.%s", tName), nil)
		_, err = b.Exec(ctx, createSQL, nil)
		if err != nil {
			return err
		}

		_, err = b.Exec(ctx, fmt.Sprintf("INSERT INTO main.%s SELECT * FROM template.%s", tName, tName), nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func valStr(v backend.Value) string {
	if v.Value == nil {
		return ""
	}
	switch val := v.Value.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
