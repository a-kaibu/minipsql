package compat

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	paramCastRE = regexp.MustCompile(`\$([0-9]+)::([A-Za-z_][A-Za-z0-9_]*)`)
	paramRE     = regexp.MustCompile(`\$([0-9]+)`)
	serialRE    = regexp.MustCompile(`(?i)\bserial\b`)
	boolRE      = regexp.MustCompile(`(?i)\bboolean\b|\bbool\b`)
	nowRE       = regexp.MustCompile(`(?i)\bnow\(\)`)
)

func Rewrite(sql string) string {
	out := paramCastRE.ReplaceAllString(sql, `cast(? as $2)`)
	out = paramRE.ReplaceAllString(out, "?")
	out = serialRE.ReplaceAllString(out, "integer")
	out = boolRE.ReplaceAllString(out, "integer")
	out = nowRE.ReplaceAllString(out, "datetime('now')")
	return out
}

func SplitStatements(sql string) []string {
	var stmts []string
	var b strings.Builder
	inSingle := false
	escaped := false
	for _, r := range sql {
		if r == '\'' && !escaped {
			inSingle = !inSingle
		}
		if r == ';' && !inSingle {
			if s := strings.TrimSpace(b.String()); s != "" {
				stmts = append(stmts, s)
			}
			b.Reset()
			escaped = false
			continue
		}
		b.WriteRune(r)
		escaped = r == '\\' && !escaped
		if r != '\\' {
			escaped = false
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

func IsQuery(sql string) bool {
	first := firstWord(sql)
	switch strings.ToLower(first) {
	case "select", "with", "pragma":
		return true
	default:
		return false
	}
}

func TxCommand(sql string) string {
	q := strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	lower := strings.ToLower(q)
	switch lower {
	case "begin", "begin transaction", "start transaction":
		return "begin"
	case "commit", "end":
		return "commit"
	case "rollback", "abort":
		return "rollback"
	default:
		return ""
	}
}

func firstWord(sql string) string {
	s := strings.TrimSpace(sql)
	for i, r := range s {
		if unicode.IsSpace(r) || r == ';' {
			return s[:i]
		}
	}
	return s
}
