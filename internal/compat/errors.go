package compat

import "strings"

type PGError struct {
	Code    string
	Message string
}

func Error(err error) PGError {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
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
