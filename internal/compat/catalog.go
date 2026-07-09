package compat

import (
	"context"
	"regexp"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
)

var joinRE = regexp.MustCompile(`(?i)\b(join|left\s+outer)\b`)

// simpleFromRE matches queries that reference a pg_catalog table in the main FROM clause
// (not in a subquery), without JOIN or comma-joins.
var (
	pgNamespaceSimpleRE  = regexp.MustCompile(`(?i)\bfrom\s+pg_catalog\.pg_namespace\b`)
	pgTypeSimpleRE       = regexp.MustCompile(`(?i)\bfrom\s+pg_catalog\.pg_type\b`)
	pgTablesSimpleRE     = regexp.MustCompile(`(?i)\bfrom\s+pg_catalog\.pg_tables\b`)
)

func tryPGCatalog(ctx context.Context, b backend.Backend, sql string, info SessionInfo) (*backend.Result, bool, error) {
	_ = info
	lower := strings.ToLower(strings.TrimSpace(sql))
	if joinRE.MatchString(lower) {
		return nil, false, nil
	}
	// Also skip if comma-joins are present (e.g. FROM pg_collation c, pg_type t)
	if strings.Contains(lower, "pg_catalog.pg_collation") || strings.Contains(lower, "pg_catalog.pg_attrdef") {
		return nil, false, nil
	}
	switch {
	case strings.Contains(lower, "pg_init_privs"):
		return pgInitPrivs(lower), true, nil
	case strings.Contains(lower, "pg_shmem_allocations_numa"):
		return pgShmemAllocationsNuma(lower), true, nil
	case pgNamespaceSimpleRE.MatchString(lower):
		return pgNamespace(), true, nil
	case pgTypeSimpleRE.MatchString(lower):
		return pgType(), true, nil
	case pgTablesSimpleRE.MatchString(lower):
		return pgTables(ctx, b, info)
	// pg_class and pg_attribute are handled by SQLite's main.pg_class/main.pg_attribute views,
	// which have all columns and consistent OIDs. Don't intercept here.
	default:
		return nil, false, nil
	}
}

func pgInitPrivs(sql string) *backend.Result {
	if strings.Contains(sql, "count(*)") {
		return &backend.Result{
			Columns: []backend.Column{{Name: "?column?", TypeOID: OIDBool}},
			Rows: [][]backend.Value{{
				{Kind: backend.ValueBool, Value: true},
			}},
		}
	}
	return &backend.Result{
		Columns: textColumns("objoid", "classoid", "objsubid", "privtype", "initprivs"),
		Rows: [][]backend.Value{{
			textValue("0"),
			textValue("0"),
			textValue("0"),
			textValue("i"),
			textValue("{}"),
		}},
	}
}

func pgShmemAllocationsNuma(sql string) *backend.Result {
	if strings.Contains(sql, "count(*)") {
		return &backend.Result{
			Columns: []backend.Column{{Name: "ok", TypeOID: OIDBool}},
			Rows: [][]backend.Value{{
				{Kind: backend.ValueBool, Value: true},
			}},
		}
	}
	return &backend.Result{
		Columns: textColumns("name", "size", "numa_node"),
	}
}

func pgNamespace() *backend.Result {
	return &backend.Result{
		Columns: []backend.Column{
			{Name: "oid", TypeOID: OIDInt4},
			{Name: "nspname", TypeOID: OIDText},
		},
		Rows: [][]backend.Value{{
			{Kind: backend.ValueInt, Value: int64(2200)},
			textValue("public"),
		}},
	}
}

func pgType() *backend.Result {
	result := &backend.Result{
		Columns: []backend.Column{
			{Name: "oid", TypeOID: OIDInt4},
			{Name: "typname", TypeOID: OIDText},
		},
	}
	for _, typ := range []struct {
		oid  uint32
		name string
	}{
		{OIDBool, "bool"},
		{OIDBytea, "bytea"},
		{OIDInt8, "int8"},
		{OIDInt2, "int2"},
		{OIDInt4, "int4"},
		{OIDText, "text"},
		{OIDFloat4, "float4"},
		{OIDFloat8, "float8"},
		{OIDVarchar, "varchar"},
		{OIDDate, "date"},
		{OIDTimestamp, "timestamp"},
		{OIDTimestamptz, "timestamptz"},
		{OIDNumeric, "numeric"},
		{OIDJSON, "json"},
		{OIDJSONB, "jsonb"},
		{OIDUUID, "uuid"},
	} {
		result.Rows = append(result.Rows, []backend.Value{
			{Kind: backend.ValueInt, Value: int64(typ.oid)},
			textValue(typ.name),
		})
	}
	return result
}

func pgTables(ctx context.Context, b backend.Backend, info SessionInfo) (*backend.Result, bool, error) {
	tables, err := sqliteTables(ctx, b)
	if err != nil {
		return nil, true, err
	}
	result := &backend.Result{
		Columns: textColumns("schemaname", "tablename", "tableowner", "tablespace", "hasindexes", "hasrules", "hastriggers", "rowsecurity"),
	}
	for _, table := range tables {
		result.Rows = append(result.Rows, []backend.Value{
			textValue("public"),
			textValue(table),
			textValue(info.User),
			{Kind: backend.ValueNull},
			textValue("f"),
			textValue("f"),
			textValue("f"),
			textValue("f"),
		})
	}
	return result, true, nil
}

func pgClass(ctx context.Context, b backend.Backend) (*backend.Result, bool, error) {
	tables, err := sqliteTables(ctx, b)
	if err != nil {
		return nil, true, err
	}
	result := &backend.Result{
		Columns: []backend.Column{
			{Name: "oid", TypeOID: OIDInt4},
			{Name: "relname", TypeOID: OIDText},
			{Name: "relnamespace", TypeOID: OIDInt4},
			{Name: "relkind", TypeOID: OIDText},
		},
	}
	for i, table := range tables {
		result.Rows = append(result.Rows, []backend.Value{
			{Kind: backend.ValueInt, Value: int64(tableOID(i))},
			textValue(table),
			{Kind: backend.ValueInt, Value: int64(2200)},
			textValue("r"),
		})
	}
	return result, true, nil
}

func pgAttribute(ctx context.Context, b backend.Backend) (*backend.Result, bool, error) {
	tables, err := sqliteTables(ctx, b)
	if err != nil {
		return nil, true, err
	}
	result := &backend.Result{
		Columns: []backend.Column{
			{Name: "attrelid", TypeOID: OIDInt4},
			{Name: "attname", TypeOID: OIDText},
			{Name: "atttypid", TypeOID: OIDInt4},
			{Name: "attnum", TypeOID: OIDInt2},
			{Name: "attnotnull", TypeOID: OIDBool},
		},
	}
	for i, table := range tables {
		cols, err := sqliteColumns(ctx, b, table)
		if err != nil {
			return nil, true, err
		}
		for _, col := range cols {
			result.Rows = append(result.Rows, []backend.Value{
				{Kind: backend.ValueInt, Value: int64(tableOID(i))},
				textValue(col.Name),
				{Kind: backend.ValueInt, Value: int64(TypeOID(col.Type))},
				{Kind: backend.ValueInt, Value: int64(col.Ordinal)},
				{Kind: backend.ValueBool, Value: col.NotNull},
			})
		}
	}
	return result, true, nil
}

func tableOID(index int) int {
	return 10000 + index
}
