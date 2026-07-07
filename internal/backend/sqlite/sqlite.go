package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/compat"
	_ "modernc.org/sqlite"
)

type Backend struct {
	db *sql.DB
	tx *sql.Tx
}

func New(ctx context.Context) (*Backend, error) {
	db, err := sql.Open("sqlite", "file:minipsql?mode=memory&cache=private")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := initDatabase(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize postgres catalog tables: %w", err)
	}
	return &Backend{db: db}, nil
}

func initDatabase(ctx context.Context, db *sql.DB) error {
	// Create catalog tables directly in main schema
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS main.pg_namespace (
			oid INTEGER PRIMARY KEY,
			nspname TEXT,
			nspowner INTEGER,
			nspacl TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_type (
			oid INTEGER PRIMARY KEY,
			typname TEXT,
			typnamespace INTEGER,
			typowner INTEGER,
			typlen INTEGER,
			typbyval INTEGER,
			typtype TEXT,
			typcategory TEXT,
			typispreferred INTEGER,
			typisdefined INTEGER,
			typdelim TEXT,
			typrelid INTEGER,
			typelem INTEGER,
			typarray INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_index (
			indexrelid INTEGER PRIMARY KEY,
			indrelid INTEGER,
			indnatts INTEGER,
			indnkeyatts INTEGER,
			indisunique INTEGER,
			indisprimary INTEGER,
			indisexclusion INTEGER,
			indimmediate INTEGER,
			indisclustered INTEGER,
			indisvalid INTEGER,
			indcheckxmin INTEGER,
			indisready INTEGER,
			indislive INTEGER,
			indisreplident INTEGER,
			indkey TEXT,
			indcollation TEXT,
			indclass TEXT,
			indoption TEXT,
			indexprs TEXT,
			indpred TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_constraint (
			oid INTEGER PRIMARY KEY,
			conname TEXT,
			connamespace INTEGER,
			contype TEXT,
			condeferrable INTEGER,
			condeferred INTEGER,
			convalidated INTEGER,
			conrelid INTEGER,
			contypid INTEGER,
			conindid INTEGER,
			conparentid INTEGER,
			confrelid INTEGER,
			confupdtype TEXT,
			confdeltype TEXT,
			confmatchtype TEXT,
			conislocal INTEGER,
			coninhcount INTEGER,
			connoinherit INTEGER,
			conkey TEXT,
			confkey TEXT,
			conpfeqop TEXT,
			conppeqop TEXT,
			conffeqop TEXT,
			conexclop TEXT,
			conbin TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_depend (
			classid INTEGER,
			objid INTEGER,
			objsubid INTEGER,
			refclassid INTEGER,
			refobjid INTEGER,
			refobjsubid INTEGER,
			deptype TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_shdepend (
			dbid INTEGER,
			classid INTEGER,
			objid INTEGER,
			objsubid INTEGER,
			refclassid INTEGER,
			refobjid INTEGER,
			deptype TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_init_privs (
			objoid INTEGER,
			classid INTEGER,
			objsubid INTEGER,
			privtype TEXT,
			initprivs TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_shmem_allocations_numa (
			name TEXT,
			size INTEGER,
			numa_node INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_proc (
			oid INTEGER PRIMARY KEY,
			proname TEXT,
			pronamespace INTEGER,
			proowner INTEGER,
			prolang INTEGER,
			procost REAL,
			prorows REAL,
			provariadic INTEGER,
			prosupport INTEGER,
			prokind TEXT,
			prosecdef INTEGER,
			proleakproof INTEGER,
			proisstrict INTEGER,
			proretset INTEGER,
			provolatile TEXT,
			proparallel TEXT,
			pronargs INTEGER,
			pronargdefaults INTEGER,
			prorettype INTEGER,
			proargtypes TEXT,
			proallargtypes TEXT,
			proargmodes TEXT,
			proargnames TEXT,
			proargdefaults TEXT,
			protrftypes TEXT,
			prosrc TEXT,
			probin TEXT,
			prosqlbody TEXT,
			proconfig TEXT,
			proacl TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_am (
			oid INTEGER PRIMARY KEY,
			amname TEXT,
			amhandler INTEGER,
			amtype TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_opclass (
			oid INTEGER PRIMARY KEY,
			opcmethod INTEGER,
			opcname TEXT,
			opcnamespace INTEGER,
			opcowner INTEGER,
			opcfamily INTEGER,
			opcintype INTEGER,
			opcdefault INTEGER,
			opckeytype INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_operator (
			oid INTEGER PRIMARY KEY,
			oprname TEXT,
			oprnamespace INTEGER,
			oprowner INTEGER,
			oprkind TEXT,
			oprcanmerge INTEGER,
			oprcanhash INTEGER,
			oprleft INTEGER,
			oprright INTEGER,
			oprresult INTEGER,
			oprcom INTEGER,
			oprnegate INTEGER,
			oprcode TEXT,
			oprrest TEXT,
			oprjoin TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_database (
			oid INTEGER PRIMARY KEY,
			datname TEXT,
			datdba INTEGER,
			encoding INTEGER,
			datcollate TEXT,
			datctype TEXT,
			datistemplate INTEGER,
			datallowconn INTEGER,
			datconnlimit INTEGER,
			datlastsysoid INTEGER,
			datfrozenxid INTEGER,
			datminmxid INTEGER,
			dattablespace INTEGER,
			datacl TEXT,
			dathasloginevt INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_collation (
			oid INTEGER PRIMARY KEY,
			collname TEXT,
			collnamespace INTEGER,
			collowner INTEGER,
			collprovider TEXT,
			collisascii INTEGER,
			collencoding INTEGER,
			collcollate TEXT,
			collctype TEXT,
			colliculocale TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_authid (
			rolname TEXT PRIMARY KEY,
			rolpassword TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_prepared_statements (
			name TEXT,
			statement TEXT,
			prepare_time TEXT,
			parameter_types TEXT,
			result_types TEXT,
			from_sql INTEGER,
			generic_plans INTEGER,
			custom_plans INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_locks (
			locktype TEXT,
			database INTEGER,
			relation INTEGER,
			page INTEGER,
			tuple INTEGER,
			virtualxid TEXT,
			transactionid INTEGER,
			classid INTEGER,
			objid INTEGER,
			objsubid INTEGER,
			virtualtransaction TEXT,
			pid INTEGER,
			mode TEXT,
			granted INTEGER,
			fastpath INTEGER,
			waitstart TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_activity (
			datid INTEGER,
			datname TEXT,
			pid INTEGER,
			leader_pid INTEGER,
			usesysid INTEGER,
			usename TEXT,
			application_name TEXT,
			client_addr TEXT,
			client_hostname TEXT,
			client_port INTEGER,
			backend_start TEXT,
			xact_start TEXT,
			query_start TEXT,
			state_change TEXT,
			wait_event_type TEXT,
			wait_event TEXT,
			state TEXT,
			backend_xid INTEGER,
			backend_xmin INTEGER,
			query_id INTEGER,
			query TEXT,
			backend_type TEXT
		)`,
	}

	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}

	// Create dynamic views in main schema
	views := []string{
		`CREATE VIEW IF NOT EXISTS main.pg_class AS
		SELECT 
			10000 + rowid AS oid,
			name AS relname,
			2200 AS relnamespace,
			0 AS reltype,
			0 AS reloftype,
			10 AS relowner,
			0 AS relam,
			0 AS relfilenode,
			0 AS reltablespace,
			0 AS relpages,
			0.0 AS reltuples,
			0 AS relallvisible,
			0 AS reltoastrelid,
			0 AS relhasindex,
			0 AS relisshared,
			'p' AS relpersistence,
			'r' AS relkind,
			0 AS relnatts,
			0 AS relchecks,
			0 AS relhasrules,
			0 AS relhastriggers,
			0 AS relhassubclass,
			0 AS relrowsecurity,
			0 AS relforcerowsecurity,
			0 AS relispopulated,
			'd' AS relreplident,
			0 AS relispartition,
			0 AS relrewrite,
			0 AS relfrozenxid,
			0 AS relminmxid
		FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,

		`CREATE VIEW IF NOT EXISTS main.pg_attribute AS
		SELECT 
			10000 + t.rowid AS attrelid,
			p.name AS attname,
			CASE 
				WHEN lower(p.type) = 'integer' THEN 23
				WHEN lower(p.type) = 'text' THEN 25
				ELSE 25
			END AS atttypid,
			-1 AS attlen,
			p.cid + 1 AS attnum,
			0 AS attcacheoff,
			-1 AS atttypmod,
			0 AS attbyval,
			'p' AS attstorage,
			'i' AS attalign,
			p."notnull" AS attnotnull,
			0 AS atthasdef,
			0 AS atthasmissing,
			'' AS attidentity,
			'' AS attgenerated,
			0 AS attisdropped,
			1 AS attislocal,
			0 AS attinhcount,
			0 AS attcollation
		FROM sqlite_schema t
		JOIN pragma_table_info(t.name) p
		WHERE t.type = 'table' AND t.name NOT LIKE 'sqlite_%'`,

		`CREATE VIEW IF NOT EXISTS main.pg_tables AS
		SELECT 
			'public' AS schemaname,
			name AS tablename,
			'mini' AS tableowner,
			NULL AS tablespace,
			0 AS hasindexes,
			0 AS hasrules,
			0 AS hastriggers,
			0 AS rowsecurity
		FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	}

	for _, view := range views {
		if _, err := db.ExecContext(ctx, view); err != nil {
			return err
		}
	}

	// Populate initial metadata
	populateSQL := `
		INSERT OR IGNORE INTO main.pg_namespace (oid, nspname) VALUES (2200, 'public');
		INSERT OR IGNORE INTO main.pg_namespace (oid, nspname) VALUES (11, 'pg_catalog');

		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (16, 'bool', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (17, 'bytea', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (20, 'int8', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (21, 'int2', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (23, 'int4', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (25, 'text', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (700, 'float4', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (701, 'float8', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1043, 'varchar', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1082, 'date', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1114, 'timestamp', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1184, 'timestamptz', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1700, 'numeric', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (114, 'json', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (3802, 'jsonb', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (2950, 'uuid', 11);

		INSERT OR IGNORE INTO main.pg_init_privs (objoid, classid, objsubid, privtype, initprivs) VALUES (0, 0, 0, 'i', '{}');
	`
	if _, err := db.ExecContext(ctx, populateSQL); err != nil {
		return err
	}

	return nil
}

// NewSession returns a per-connection Backend that shares the underlying
// *sql.DB but has its own transaction state. Close is a no-op; the DB
// lifetime is managed by the owner Backend.
func (b *Backend) NewSession() backend.Backend {
	return &sessionBackend{db: b.db}
}

// sessionBackend wraps a shared *sql.DB with per-session transaction state.
type sessionBackend struct {
	db *sql.DB
	tx *sql.Tx
}

func (s *sessionBackend) queryer() interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *sessionBackend) execer() interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *sessionBackend) Query(ctx context.Context, query string, args []backend.Value) (*backend.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.queryer().QueryContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, _ := rows.ColumnTypes()
	cols := make([]backend.Column, len(names))
	for i, name := range names {
		dbType := ""
		if i < len(types) {
			dbType = types[i].DatabaseTypeName()
		}
		cols[i] = backend.Column{Name: name, TypeOID: compat.TypeOID(dbType)}
	}
	var out [][]backend.Value
	for rows.Next() {
		raw := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]backend.Value, len(raw))
		for i, v := range raw {
			row[i] = valueFromDB(v)
			if cols[i].TypeOID == compat.OIDText && row[i].Kind != backend.ValueNull {
				cols[i].TypeOID = oidFromValue(row[i])
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &backend.Result{Columns: cols, Rows: out}, nil
}

func (s *sessionBackend) Exec(ctx context.Context, query string, args []backend.Value) (*backend.CommandTag, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := s.execer().ExecContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	return &backend.CommandTag{Tag: commandTag(query, n)}, nil
}

func (s *sessionBackend) Begin(ctx context.Context) error {
	if s.tx != nil {
		return errors.New("transaction already in progress")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	s.tx = tx
	return nil
}

func (s *sessionBackend) Commit(context.Context) error {
	if s.tx == nil {
		return errors.New("no transaction in progress")
	}
	err := s.tx.Commit()
	s.tx = nil
	return err
}

func (s *sessionBackend) Rollback(context.Context) error {
	if s.tx == nil {
		return errors.New("no transaction in progress")
	}
	err := s.tx.Rollback()
	s.tx = nil
	return err
}

func (s *sessionBackend) Close(context.Context) error { return nil }

func (s *sessionBackend) BulkInsert(ctx context.Context, table string, columns []string, rows [][]backend.Value) (int64, error) {
	b := &Backend{db: s.db, tx: s.tx}
	return b.BulkInsert(ctx, table, columns, rows)
}

func (s *sessionBackend) TableColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := s.queryer().QueryContext(ctx, "PRAGMA table_info(\""+table+"\")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("relation %q does not exist", table)
	}
	return cols, nil
}

func (b *Backend) Query(ctx context.Context, query string, args []backend.Value) (*backend.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := b.queryer().QueryContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, _ := rows.ColumnTypes()
	cols := make([]backend.Column, len(names))
	for i, name := range names {
		dbType := ""
		if i < len(types) {
			dbType = types[i].DatabaseTypeName()
		}
		cols[i] = backend.Column{Name: name, TypeOID: compat.TypeOID(dbType), Format: 0}
	}

	var out [][]backend.Value
	for rows.Next() {
		raw := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]backend.Value, len(raw))
		for i, v := range raw {
			row[i] = valueFromDB(v)
			if cols[i].TypeOID == compat.OIDText && row[i].Kind != backend.ValueNull {
				cols[i].TypeOID = oidFromValue(row[i])
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &backend.Result{Columns: cols, Rows: out}, nil
}

func (b *Backend) Exec(ctx context.Context, query string, args []backend.Value) (*backend.CommandTag, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := b.execer().ExecContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	return &backend.CommandTag{Tag: commandTag(query, n)}, nil
}

func (b *Backend) Begin(ctx context.Context) error {
	if b.tx != nil {
		return errors.New("transaction already in progress")
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	b.tx = tx
	return nil
}

func (b *Backend) Commit(ctx context.Context) error {
	if b.tx == nil {
		return errors.New("no transaction in progress")
	}
	err := b.tx.Commit()
	b.tx = nil
	return err
}

func (b *Backend) Rollback(ctx context.Context) error {
	if b.tx == nil {
		return errors.New("no transaction in progress")
	}
	err := b.tx.Rollback()
	b.tx = nil
	return err
}

func (b *Backend) Close(context.Context) error {
	return b.db.Close()
}

func (b *Backend) queryer() interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
} {
	if b.tx != nil {
		return b.tx
	}
	return b.db
}

func (b *Backend) execer() interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
} {
	if b.tx != nil {
		return b.tx
	}
	return b.db
}

func sqlArgs(vals []backend.Value) []any {
	args := make([]any, len(vals))
	for i, v := range vals {
		switch v.Kind {
		case backend.ValueNull:
			args[i] = nil
		default:
			args[i] = v.Value
		}
	}
	return args
}

func valueFromDB(v any) backend.Value {
	switch x := v.(type) {
	case nil:
		return backend.Value{Kind: backend.ValueNull}
	case bool:
		return backend.Value{Kind: backend.ValueBool, Value: x}
	case int64:
		return backend.Value{Kind: backend.ValueInt, Value: x}
	case int:
		return backend.Value{Kind: backend.ValueInt, Value: int64(x)}
	case float64:
		return backend.Value{Kind: backend.ValueFloat, Value: x}
	case []byte:
		return backend.Value{Kind: backend.ValueBytes, Value: x}
	case string:
		return backend.Value{Kind: backend.ValueText, Value: x}
	default:
		return backend.Value{Kind: backend.ValueText, Value: fmt.Sprint(x)}
	}
}

func oidFromValue(v backend.Value) uint32 {
	switch v.Kind {
	case backend.ValueBool:
		return compat.OIDBool
	case backend.ValueInt:
		return compat.OIDInt8
	case backend.ValueFloat:
		return compat.OIDFloat8
	case backend.ValueBytes:
		return compat.OIDBytea
	default:
		return compat.OIDText
	}
}

func commandTag(query string, affected int64) string {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return "OK"
	}
	switch strings.ToUpper(fields[0]) {
	case "INSERT":
		return "INSERT 0 " + strconv.FormatInt(affected, 10)
	case "UPDATE":
		return "UPDATE " + strconv.FormatInt(affected, 10)
	case "DELETE":
		return "DELETE " + strconv.FormatInt(affected, 10)
	case "CREATE":
		if len(fields) > 1 {
			return "CREATE " + strings.ToUpper(fields[1])
		}
		return "CREATE"
	case "DROP":
		if len(fields) > 1 {
			return "DROP " + strings.ToUpper(fields[1])
		}
		return "DROP"
	default:
		return strings.ToUpper(fields[0])
	}
}

func (b *Backend) TableColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := b.queryer().QueryContext(ctx, "PRAGMA table_info(\""+table+"\")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("relation %q does not exist", table)
	}
	return cols, nil
}

func (b *Backend) BulkInsert(ctx context.Context, table string, columns []string, rows [][]backend.Value) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	if len(columns) == 0 {
		cols, err := b.TableColumns(ctx, table)
		if err != nil {
			return 0, err
		}
		columns = cols
	}

	colList := make([]string, len(columns))
	for i, c := range columns {
		colList[i] = "\"" + c + "\""
	}
	colSQL := strings.Join(colList, ", ")

	ncols := len(columns)
	placeholders := "(" + strings.Repeat("?, ", ncols-1) + "?)"

	const batchSize = 200
	var total int64
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO \"")
		sb.WriteString(table)
		sb.WriteString("\" (")
		sb.WriteString(colSQL)
		sb.WriteString(") VALUES ")
		for i := range batch {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(placeholders)
		}

		args := make([]any, 0, len(batch)*ncols)
		for _, row := range batch {
			for j := 0; j < ncols; j++ {
				if j < len(row) {
					switch row[j].Kind {
					case backend.ValueNull:
						args = append(args, nil)
					default:
						args = append(args, row[j].Value)
					}
				} else {
					args = append(args, nil)
				}
			}
		}

		res, err := b.execer().ExecContext(ctx, sb.String(), args...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}
