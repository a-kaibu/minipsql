package compat

import (
	"strconv"
	"strings"
)

type PGError struct {
	Code    string
	Message string
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
	case strings.Contains(lower, "constraint failed") || strings.Contains(lower, "unique constraint"):
		return PGError{Code: "23505", Message: msg, Hint: hint}
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
