package compat

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	paramCastRE   = regexp.MustCompile(`\$([0-9]+)::([A-Za-z_][A-Za-z0-9_]*)`)
	paramRE       = regexp.MustCompile(`\$([0-9]+)`)
	serialRE      = regexp.MustCompile(`(?i)\bserial\b`)
	boolRE        = regexp.MustCompile(`(?i)\bboolean\b|\bbool\b`)
	nowRE         = regexp.MustCompile(`(?i)\bnow\(\)`)
	castRE        = regexp.MustCompile(`::([A-Za-z_][A-Za-z0-9_ ]*(?:\([^)]*\))?(?:\[\])?)`)
	ilikeRE       = regexp.MustCompile(`(?i)\bILIKE\b`)
	returningRE   = regexp.MustCompile(`(?i)\bRETURNING\b`)
	onlyRE        = regexp.MustCompile(`(?i)\bFROM\s+ONLY\s+`)
	eStringRE     = regexp.MustCompile(`(?i)\bE'`)
	trueRE        = regexp.MustCompile(`(?i)\bTRUE\b`)
	falseRE       = regexp.MustCompile(`(?i)\bFALSE\b`)
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
	out = rewritePosition(out)
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
	for i > 0 && (isIdentByte(sql[i-1]) || sql[i-1] == '.') {
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
	inLineComment := false
	blockDepth := 0
	dollarTag := ""
	for i := 0; i < len(sql); i++ {
		if inLineComment {
			if sql[i] == '\n' {
				inLineComment = false
				b.WriteByte('\n')
			}
			continue
		}

		if blockDepth > 0 {
			if i+1 < len(sql) && sql[i] == '/' && sql[i+1] == '*' {
				blockDepth++
				i++
				continue
			}
			if i+1 < len(sql) && sql[i] == '*' && sql[i+1] == '/' {
				blockDepth--
				i++
				if blockDepth == 0 {
					b.WriteByte(' ')
				}
			}
			continue
		}

		if dollarTag != "" {
			if strings.HasPrefix(sql[i:], dollarTag) {
				b.WriteString(dollarTag)
				i += len(dollarTag) - 1
				dollarTag = ""
				continue
			}
			b.WriteByte(sql[i])
			continue
		}

		if inSingle {
			b.WriteByte(sql[i])
			if sql[i] == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					b.WriteByte(sql[i+1])
					i++
					continue
				}
				inSingle = false
			}
			continue
		}

		if i+1 < len(sql) && sql[i] == '-' && sql[i+1] == '-' {
			inLineComment = true
			i++
			continue
		}
		if i+1 < len(sql) && sql[i] == '/' && sql[i+1] == '*' {
			blockDepth = 1
			i++
			continue
		}
		if sql[i] == '\'' {
			inSingle = true
			b.WriteByte(sql[i])
			continue
		}
		if tag := dollarQuoteTagAt(sql, i); tag != "" {
			dollarTag = tag
			b.WriteString(tag)
			i += len(tag) - 1
			continue
		}

		if sql[i] == ';' {
			if s := strings.TrimSpace(b.String()); s != "" {
				stmts = append(stmts, s)
			}
			b.Reset()
			continue
		}
		b.WriteByte(sql[i])
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

func dollarQuoteTagAt(sql string, start int) string {
	if start >= len(sql) || sql[start] != '$' {
		return ""
	}
	end := start + 1
	for end < len(sql) {
		c := sql[end]
		if c == '$' {
			return sql[start : end+1]
		}
		if !(isIdentByte(c)) {
			return ""
		}
		end++
	}
	return ""
}

func rewritePosition(sql string) string {
	result := sql
	for {
		idx := indexOutsideQuotes(strings.ToUpper(result), "POSITION")
		if idx < 0 {
			return result
		}
		open := idx + len("POSITION")
		for open < len(result) && unicode.IsSpace(rune(result[open])) {
			open++
		}
		if open >= len(result) || result[open] != '(' {
			return result
		}
		close := findMatchingParen(result, open)
		if close < 0 {
			return result
		}
		body := result[open+1 : close]
		inStart, inEnd := topLevelInBounds(body)
		if inStart < 0 {
			idx = close + 1
			return result[:idx] + rewritePosition(result[idx:])
		}
		needle := strings.TrimSpace(body[:inStart])
		haystack := strings.TrimSpace(body[inEnd:])
		replacement := "instr(" + haystack + ", " + needle + ")"
		result = result[:idx] + replacement + result[close+1:]
	}
}

func findMatchingParen(sql string, open int) int {
	depth := 1
	inSingle := false
	for i := open + 1; i < len(sql); i++ {
		if inSingle {
			if sql[i] == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		switch sql[i] {
		case '\'':
			inSingle = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func topLevelInBounds(s string) (int, int) {
	depth := 0
	inSingle := false
	for i := 0; i < len(s); i++ {
		if inSingle {
			if s[i] == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		switch s[i] {
		case '\'':
			inSingle = true
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && (s[i] == 'I' || s[i] == 'i') && i+1 < len(s) && (s[i+1] == 'N' || s[i+1] == 'n') {
				beforeOK := i == 0 || unicode.IsSpace(rune(s[i-1]))
				after := i + len("in")
				afterOK := after >= len(s) || unicode.IsSpace(rune(s[after]))
				if beforeOK && afterOK {
					return i, after
				}
			}
		}
	}
	return -1, -1
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
