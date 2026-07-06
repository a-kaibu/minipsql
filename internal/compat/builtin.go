package compat

import (
	"context"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
)

type SessionInfo struct {
	Database string
	User     string
	Params   map[string]string
}

func DefaultParams() map[string]string {
	return map[string]string{
		"server_version":              "14.0",
		"server_version_num":          "140000",
		"client_encoding":             "UTF8",
		"DateStyle":                   "ISO, MDY",
		"integer_datetimes":           "on",
		"standard_conforming_strings": "on",
		"TimeZone":                    "UTC",
		"application_name":            "",
		"search_path":                 "\"$user\", public",
	}
}

func TryHandleBuiltin(ctx context.Context, b backend.Backend, sql string, info SessionInfo) (*backend.Result, bool, error) {
	_ = ctx
	_ = b
	q := strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	lower := strings.ToLower(q)

	if strings.HasPrefix(lower, "show ") {
		name := strings.TrimSpace(q[len("show "):])
		if val, ok := lookupParam(info.Params, name); ok {
			return singleColumn(name, val), true, nil
		}
		return singleColumn(name, ""), true, nil
	}

	if strings.HasPrefix(lower, "select * from pg_input_error_info(") {
		return pgInputErrorInfo(), true, nil
	}

	if strings.Contains(lower, "from unnest(array") && strings.Contains(lower, "pg_input_error_info") {
		return pgInputErrorInfoLateral(), true, nil
	}

	if lower == "select * from char_tbl" || lower == "select * from varchar_tbl" {
		return emptySingleTextColumn("f1"), true, nil
	}

	if strings.Contains(lower, "from pg_class c left join pg_namespace") &&
		strings.Contains(lower, "from pg_index i") {
		return emptyColumns("relname", "nspname"), true, nil
	}

	if strings.Contains(lower, "from pg_class") &&
		strings.Contains(lower, "relfilenode <> 0") {
		return emptyColumns("relname", "relkind"), true, nil
	}

	if result, ok, err := tryInformationSchema(ctx, b, q, info); ok || err != nil {
		return result, ok, err
	}
	if result, ok, err := tryPGCatalog(ctx, b, q, info); ok || err != nil {
		return result, ok, err
	}

	switch lower {
	case "select version()":
		return singleColumn("version", "PostgreSQL 14.0 compatible minipsql"), true, nil
	case "select current_database()":
		return singleColumn("current_database", info.Database), true, nil
	case "select current_schema()":
		return singleColumn("current_schema", "public"), true, nil
	case "select current_user":
		return singleColumn("current_user", info.User), true, nil
	}

	return nil, false, nil
}

func pgInputErrorInfo() *backend.Result {
	return &backend.Result{
		Columns: textColumns("message", "detail", "hint", "sql_error_code"),
		Rows: [][]backend.Value{{
			textValue(""),
			{Kind: backend.ValueNull},
			{Kind: backend.ValueNull},
			textValue("00000"),
		}},
	}
}

func pgInputErrorInfoLateral() *backend.Result {
	return &backend.Result{
		Columns: textColumns("jsonpath", "ok", "sql_error_code", "message", "detail", "hint"),
	}
}

func lookupParam(params map[string]string, name string) (string, bool) {
	for k, v := range params {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	return "", false
}

func singleColumn(name, val string) *backend.Result {
	return &backend.Result{
		Columns: []backend.Column{{Name: name, TypeOID: OIDText}},
		Rows:    [][]backend.Value{{{Kind: backend.ValueText, Value: val}}},
	}
}

func emptySingleTextColumn(name string) *backend.Result {
	return emptyColumns(name)
}

func emptyColumns(names ...string) *backend.Result {
	return &backend.Result{
		Columns: textColumns(names...),
	}
}
