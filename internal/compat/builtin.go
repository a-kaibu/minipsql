package compat

import (
	"context"
	"strings"

	"github.com/yashikota/minipostgresql/internal/backend"
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
