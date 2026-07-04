# minipsql

minipsql is a lightweight PostgreSQL-wire-compatible in-memory test database.
It accepts PostgreSQL client connections and uses SQLite as the execution backend.

## Usage

```bash
go run ./cmd/minipsql serve
psql -h localhost -p 15432 -U mini default -c "select 1"
```

## Supported

- PostgreSQL startup handshake with trust authentication
- Simple Query Protocol
- Basic Extended Query Protocol
- Basic SQLite-backed DDL/DML
- Minimal PostgreSQL builtins such as `version()`, `SHOW server_version`, and `current_database()`
- Basic transaction commands: `begin`, `commit`, `rollback`

## Unsupported

- PostgreSQL extension
- PL/pgSQL
- trigger
- advanced index
- advanced transaction isolation
- replication
- role/permission
- full pg_catalog compatibility
