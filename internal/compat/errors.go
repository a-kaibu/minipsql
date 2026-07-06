package compat

import (
	"strconv"
	"strings"
)

type PGError struct {
	Code    string
	Message string
}

func Error(err error) PGError {
	msg := cleanSQLiteError(err.Error())
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "invalid input syntax"):
		return PGError{Code: "22P02", Message: msg}
	case strings.Contains(lower, "syntax error"):
		return PGError{Code: "42601", Message: msg}
	case strings.Contains(lower, "no such table"):
		return PGError{Code: "42P01", Message: msg}
	case strings.Contains(lower, "no such column"):
		return PGError{Code: "42703", Message: msg}
	case strings.Contains(lower, "constraint failed") || strings.Contains(lower, "unique constraint"):
		return PGError{Code: "23505", Message: msg}
	default:
		return PGError{Code: "XX000", Message: msg}
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
