package compat

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	paramCastRE = regexp.MustCompile(`\$([0-9]+)::([A-Za-z_][A-Za-z0-9_]*)`)
	paramRE     = regexp.MustCompile(`\$([0-9]+)`)
	serialRE    = regexp.MustCompile(`(?i)\bserial\b`)
	boolRE      = regexp.MustCompile(`(?i)\bboolean\b|\bbool\b`)
	nowRE       = regexp.MustCompile(`(?i)\bnow\(\)`)
	castRE      = regexp.MustCompile(`::([A-Za-z_][A-Za-z0-9_ ]*(?:\([^)]*\))?(?:\[\])?)`)
	ilikeRE     = regexp.MustCompile(`(?i)\bILIKE\b`)
	returningRE = regexp.MustCompile(`(?i)\bRETURNING\b`)
	onlyRE      = regexp.MustCompile(`(?i)\bFROM\s+ONLY\s+`)
	eStringRE   = regexp.MustCompile(`(?i)\bE'`)
	trueRE      = regexp.MustCompile(`(?i)\bTRUE\b`)
	falseRE     = regexp.MustCompile(`(?i)\bFALSE\b`)
	ifNotExistsRE = regexp.MustCompile(`(?i)\bIF\s+NOT\s+EXISTS\b`)
	ifExistsRE    = regexp.MustCompile(`(?i)\bIF\s+EXISTS\b`)
)

func Rewrite(sql string) string {
	out := paramCastRE.ReplaceAllString(sql, `cast(? as $2)`)
	out = paramRE.ReplaceAllString(out, "?")
	out = serialRE.ReplaceAllString(out, "integer")
	out = boolRE.ReplaceAllString(out, "integer")
	out = nowRE.ReplaceAllString(out, "datetime('now')")
	out = rewritePGCast(out)
	out = ilikeRE.ReplaceAllString(out, "LIKE")
	out = rewriteRegexOps(out)
	out = rewriteSimilarTo(out)
	out = rewriteNullsOrder(out)
	out = rewriteGenerateSeries(out)
	out = onlyRE.ReplaceAllString(out, "FROM ")
	out = eStringRE.ReplaceAllString(out, "'")
	return out
}

func rewritePGCast(sql string) string {
	for {
		loc := castRE.FindStringIndex(sql)
		if loc == nil {
			break
		}
		typeName := strings.TrimSpace(castRE.FindStringSubmatch(sql[loc[0]:])[1])
		typeName = strings.TrimSuffix(typeName, "[]")
		typeName = strings.TrimSpace(typeName)
		if idx := strings.Index(typeName, "("); idx >= 0 {
			typeName = typeName[:idx]
		}
		typeName = mapPGType(typeName)
		expr := extractCastExpr(sql[:loc[0]])
		prefix := sql[:loc[0]-len(expr)]
		suffix := sql[loc[1]:]
		if expr == "" {
			sql = prefix + suffix
		} else {
			sql = prefix + "CAST(" + expr + " AS " + typeName + ")" + suffix
		}
	}
	return sql
}

func mapPGType(t string) string {
	switch strings.ToLower(t) {
	case "integer", "int", "int4", "int2", "smallint", "bigint", "int8":
		return "integer"
	case "real", "float4", "float8", "double precision", "double":
		return "real"
	case "text", "varchar", "character varying", "character", "char", "bpchar", "name":
		return "text"
	case "boolean", "bool":
		return "integer"
	case "numeric", "decimal":
		return "numeric"
	case "bytea":
		return "blob"
	case "regclass", "regtype", "oid", "regproc", "regoper":
		return "text"
	default:
		return "text"
	}
}

func extractCastExpr(sql string) string {
	if len(sql) == 0 {
		return ""
	}
	i := len(sql) - 1
	if sql[i] == '\'' {
		i--
		for i >= 0 {
			if sql[i] == '\'' {
				if i > 0 && sql[i-1] == '\'' {
					i -= 2
					continue
				}
				break
			}
			i--
		}
		if i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') {
			i--
		}
		return sql[i:]
	}
	if sql[i] == ')' {
		depth := 1
		i--
		for i >= 0 && depth > 0 {
			if sql[i] == ')' {
				depth++
			} else if sql[i] == '(' {
				depth--
			}
			i--
		}
		i++
		for i > 0 && isIdentByte(sql[i-1]) {
			i--
		}
		return sql[i:]
	}
	for i > 0 && (isIdentByte(sql[i-1]) || sql[i-1] == '.' || sql[i-1] == '-') {
		i--
	}
	return sql[i:]
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func HasReturning(sql string) bool {
	return returningRE.MatchString(sql)
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

func ParamCount(sql string) int {
	max := 0
	for _, match := range paramRE.FindAllStringSubmatch(sql, -1) {
		if len(match) < 2 {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err == nil && n > max {
			max = n
		}
	}
	return max
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
