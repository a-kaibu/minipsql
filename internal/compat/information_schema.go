package compat

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
)

func tryInformationSchema(ctx context.Context, b backend.Backend, sql string, info SessionInfo) (*backend.Result, bool, error) {
	lower := strings.ToLower(strings.TrimSpace(sql))
	switch {
	case strings.Contains(lower, "information_schema.tables"):
		return informationSchemaTables(ctx, b, info)
	case strings.Contains(lower, "information_schema.columns"):
		return informationSchemaColumns(ctx, b, info)
	default:
		return nil, false, nil
	}
}

func informationSchemaTables(ctx context.Context, b backend.Backend, info SessionInfo) (*backend.Result, bool, error) {
	tables, err := sqliteTables(ctx, b)
	if err != nil {
		return nil, true, err
	}
	result := &backend.Result{
		Columns: textColumns("table_catalog", "table_schema", "table_name", "table_type"),
	}
	for _, table := range tables {
		result.Rows = append(result.Rows, []backend.Value{
			textValue(info.Database),
			textValue("public"),
			textValue(table),
			textValue("BASE TABLE"),
		})
	}
	return result, true, nil
}

func informationSchemaColumns(ctx context.Context, b backend.Backend, info SessionInfo) (*backend.Result, bool, error) {
	tables, err := sqliteTables(ctx, b)
	if err != nil {
		return nil, true, err
	}
	result := &backend.Result{
		Columns: []backend.Column{
			{Name: "table_catalog", TypeOID: OIDText},
			{Name: "table_schema", TypeOID: OIDText},
			{Name: "table_name", TypeOID: OIDText},
			{Name: "column_name", TypeOID: OIDText},
			{Name: "ordinal_position", TypeOID: OIDInt4},
			{Name: "column_default", TypeOID: OIDText},
			{Name: "is_nullable", TypeOID: OIDText},
			{Name: "data_type", TypeOID: OIDText},
		},
	}
	for _, table := range tables {
		cols, err := sqliteColumns(ctx, b, table)
		if err != nil {
			return nil, true, err
		}
		for _, col := range cols {
			nullable := "YES"
			if col.NotNull {
				nullable = "NO"
			}
			result.Rows = append(result.Rows, []backend.Value{
				textValue(info.Database),
				textValue("public"),
				textValue(table),
				textValue(col.Name),
				{Kind: backend.ValueInt, Value: int64(col.Ordinal)},
				nullableText(col.Default),
				textValue(nullable),
				textValue(pgDataType(col.Type)),
			})
		}
	}
	return result, true, nil
}

type sqliteColumn struct {
	Ordinal int
	Name    string
	Type    string
	NotNull bool
	Default *string
}

func sqliteTables(ctx context.Context, b backend.Backend) ([]string, error) {
	result, err := b.Query(ctx, "select name from sqlite_master where type = 'table' and name not like 'sqlite_%' and name not like 'pg_%' order by name", nil)
	if err != nil {
		return nil, err
	}
	tables := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) > 0 && row[0].Kind != backend.ValueNull {
			tables = append(tables, fmt.Sprint(row[0].Value))
		}
	}
	return tables, nil
}

func sqliteColumns(ctx context.Context, b backend.Backend, table string) ([]sqliteColumn, error) {
	result, err := b.Query(ctx, "pragma table_info("+quoteIdent(table)+")", nil)
	if err != nil {
		return nil, err
	}
	cols := make([]sqliteColumn, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) < 5 {
			continue
		}
		def := (*string)(nil)
		if row[4].Kind != backend.ValueNull {
			s := fmt.Sprint(row[4].Value)
			def = &s
		}
		cols = append(cols, sqliteColumn{
			Ordinal: int(asInt(row[0])) + 1,
			Name:    fmt.Sprint(row[1].Value),
			Type:    fmt.Sprint(row[2].Value),
			NotNull: asInt(row[3]) != 0,
			Default: def,
		})
	}
	return cols, nil
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func textColumns(names ...string) []backend.Column {
	cols := make([]backend.Column, len(names))
	for i, name := range names {
		cols[i] = backend.Column{Name: name, TypeOID: OIDText}
	}
	return cols
}

func textValue(s string) backend.Value {
	return backend.Value{Kind: backend.ValueText, Value: s}
}

func nullableText(v *string) backend.Value {
	if v == nil {
		return backend.Value{Kind: backend.ValueNull}
	}
	return textValue(*v)
}

func asInt(v backend.Value) int64 {
	switch v.Kind {
	case backend.ValueInt:
		switch x := v.Value.(type) {
		case int64:
			return x
		case int:
			return int64(x)
		}
	case backend.ValueText:
		n, _ := strconv.ParseInt(fmt.Sprint(v.Value), 10, 64)
		return n
	}
	return 0
}

func pgDataType(declared string) string {
	switch TypeOID(declared) {
	case OIDBool:
		return "boolean"
	case OIDInt2:
		return "smallint"
	case OIDInt4:
		return "integer"
	case OIDInt8:
		return "bigint"
	case OIDFloat4:
		return "real"
	case OIDFloat8:
		return "double precision"
	case OIDVarchar:
		return "character varying"
	case OIDBytea:
		return "bytea"
	default:
		return "text"
	}
}
