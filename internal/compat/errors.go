package compat

import (
	"strconv"
	"strings"
)

type PGError struct {
	Code    string
	Message string
	Detail  string
	Hint    string
}

func Error(err error) PGError {
	if pg, ok := err.(*PGError); ok {
		return *pg
	}
	msg := cleanSQLiteError(err.Error())
	var hint string
	if idx := strings.Index(strings.ToLower(msg), "\nhint: "); idx >= 0 {
		hint = msg[idx+len("\nhint: "):]
		msg = msg[:idx]
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "invalid unicode escape"):
		return PGError{
			Code:    "22025",
			Message: "invalid Unicode escape",
			Hint:    "Unicode escapes must be \\XXXX, \\+XXXXXX, \\uXXXX, or \\UXXXXXXXX.",
		}
	case strings.Contains(lower, "invalid input syntax"):
		return PGError{Code: "22P02", Message: msg, Hint: hint}
	case strings.Contains(lower, "syntax error"):
		return PGError{Code: "42601", Message: msg, Hint: hint}
	case strings.Contains(lower, "no such table"):
		return PGError{Code: "42P01", Message: msg, Hint: hint}
	case strings.Contains(lower, "no such column"):
		return PGError{Code: "42703", Message: msg, Hint: hint}
	case strings.Contains(lower, "no such function"):
		return PGError{Code: "42883", Message: msg, Hint: hint}
	case strings.Contains(lower, "not null constraint failed:"):
		// SQLite: "NOT NULL constraint failed: table.col" → PG style
		pgMsg := msg
		if idx := strings.LastIndex(lower, "not null constraint failed: "); idx >= 0 {
			tableCol := msg[idx+len("NOT NULL constraint failed: "):]
			if dotIdx := strings.LastIndex(tableCol, "."); dotIdx >= 0 {
				tbl := tableCol[:dotIdx]
				col := tableCol[dotIdx+1:]
				pgMsg = `null value in column "` + col + `" of relation "` + tbl + `" violates not-null constraint`
			}
		}
		return PGError{Code: "23502", Message: pgMsg, Hint: hint}
	case strings.Contains(lower, "not null constraint"):
		return PGError{Code: "23502", Message: msg, Hint: hint}
	case strings.Contains(lower, "check constraint failed:"):
		// SQLite: "CHECK constraint failed: constraintname" → PG style
		pgMsg := msg
		if idx := strings.LastIndex(lower, "check constraint failed: "); idx >= 0 {
			cname := msg[idx+len("CHECK constraint failed: "):]
			pgMsg = `new row for relation "` + cname + `" violates check constraint "` + cname + `"`
		}
		return PGError{Code: "23514", Message: pgMsg, Hint: hint}
	case strings.Contains(lower, "check constraint"):
		return PGError{Code: "23514", Message: msg, Hint: hint}
	case strings.Contains(lower, "unique constraint") || strings.Contains(lower, "unique index"):
		return PGError{Code: "23505", Message: msg, Hint: hint}
	case strings.Contains(lower, "constraint failed"):
		return PGError{Code: "23505", Message: msg, Hint: hint}
	case strings.Contains(lower, "stack depth limit"):
		return PGError{Code: "54001", Message: "stack depth limit exceeded"}
	case strings.Contains(lower, "division by zero") || strings.Contains(lower, "divide by zero"):
		return PGError{Code: "22012", Message: "division by zero"}
	case strings.Contains(lower, "out of range") || strings.Contains(lower, "integer overflow"):
		return PGError{Code: "22003", Message: msg, Hint: hint}
	case strings.Contains(lower, "already exists"):
		return PGError{Code: "42P07", Message: msg, Hint: hint}
	case strings.Contains(lower, "ambiguous column"):
		return PGError{Code: "42702", Message: msg, Hint: hint}
	case strings.Contains(lower, "cannot modify") && strings.Contains(lower, "because it is a view"):
		// SQLite "cannot modify X because it is a view" → PG "cannot insert into view"
		name := ""
		if i := strings.Index(lower, "cannot modify "); i >= 0 {
			rest := msg[i+len("cannot modify "):]
			if end := strings.Index(rest, " because"); end >= 0 {
				name = rest[:end]
			}
		}
		return PGError{Code: "55000", Message: `cannot insert into view "` + name + `"`}
	case strings.Contains(lower, "too many") && strings.Contains(lower, "column"):
		return PGError{Code: "42601", Message: msg, Hint: hint}
	case strings.Contains(lower, "columns available but") || strings.Contains(lower, "values for"):
		return PGError{Code: "42601", Message: msg, Hint: hint}
	default:
		return PGError{Code: "XX000", Message: msg, Hint: hint}
	}
}

func (e *PGError) Error() string {
	return e.Message
}

func cleanSQLiteError(msg string) string {
	msg = strings.TrimPrefix(msg, "SQL logic error: ")
	if idx := strings.LastIndex(msg, " ("); idx >= 0 {
		tail := msg[idx+2:]
		if strings.HasSuffix(tail, ")") {
			numStr := tail[:len(tail)-1]
			if _, err := strconv.Atoi(numStr); err == nil {
				msg = msg[:idx]
			}
		}
	}
	return msg
}
