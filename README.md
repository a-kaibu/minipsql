# minipsql

[![s3-tests](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/yashikota/minipsql/main/.github/badges/compat-tests.json&cacheSeconds=300)](https://github.com/yashikota/minipsql/blob/main/.github/workflows/compat-tests.yaml)

minipsql is a lightweight PostgreSQL-wire-compatible in-memory test database.
It accepts PostgreSQL client connections and uses SQLite as the execution backend.

## Usage

```bash
go run . serve
psql -h localhost -p 15432 -U mini default -c "select 1"
```

Run schema SQL for each new in-memory database:

```bash
go run . serve --init schema.sql
```

## Compatibility Stress Tests

Initialize upstream test sources:

```bash
git submodule update --init --recursive
```

Run every SQL file under PostgreSQL's `src/test/regress/sql` against minipsql:

```bash
task regress
```

The runner generates one psql input that includes every upstream regress SQL file in sorted order.
It does not skip known-incompatible files, does not rewrite upstream SQL, and exits non-zero when psql reports any `ERROR`, `FATAL`, or `PANIC`.
The PostgreSQL client is run via the local Docker image built from `docker/postgres-regress.Dockerfile`.
Reports are written under `reports/postgres-regress/`, including total/success/failure counts and per-file results.

Run SQLancer's PostgreSQL provider against minipsql:

```bash
task sqlancer
```

SQLancer is run via the local Docker image built from `docker/sqlancer.Dockerfile`.
It runs for 600 seconds by default; override with `SQLANCER_TIMEOUT_SECONDS=<seconds>`.
Reports are written under `reports/sqlancer/`.

## Supported

- PostgreSQL startup handshake with trust authentication
- Simple Query Protocol
- Basic Extended Query Protocol
- Basic SQLite-backed DDL/DML
- Minimal PostgreSQL builtins such as `version()`, `SHOW server_version`, and `current_database()`
- Basic transaction commands: `begin`, `commit`, `rollback`
- Per-session initialization SQL with `--init`

## Unsupported

- PostgreSQL extension
- PL/pgSQL
- trigger
- advanced index
- advanced transaction isolation
- replication
- role/permission
- full pg_catalog compatibility
