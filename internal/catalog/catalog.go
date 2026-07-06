package catalog

import (
	"context"
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
	mu      sync.RWMutex
	dbs     map[string]*dbEntry
	initSQL string
}

// New creates a Catalog with a pre-initialized "default" database.
func New(ctx context.Context, initSQL string) (*Catalog, error) {
	c := &Catalog{
		dbs:     make(map[string]*dbEntry),
		initSQL: initSQL,
	}
	if err := c.createDB(ctx, "default"); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Catalog) createDB(ctx context.Context, name string) error {
	b, err := sqlite.New(ctx)
	if err != nil {
		return err
	}
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
}
