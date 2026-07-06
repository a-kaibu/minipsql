package compat

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
)

var (
	paramRE            = regexp.MustCompile(`\$([0-9]+)`)
	serialRE           = regexp.MustCompile(`(?i)\bserial\b`)
	boolRE             = regexp.MustCompile(`(?i)\bboolean\b|\bbool\b`)
	nowRE              = regexp.MustCompile(`(?i)\bnow\(\)`)
	quotedCharRE       = regexp.MustCompile(`::"char"`)
	castRE             = regexp.MustCompile(`::([A-Za-z_][A-Za-z0-9_ ]*(?:\([^)]*\))?(?:\[\])?)`)
	charLiteralRE      = regexp.MustCompile(`(?i)\bchar\s+('(?:''|[^'])*')`)
	pgLsnLiteralRE     = regexp.MustCompile(`(?i)\bpg_lsn\s+('(?:''|[^'])*')`)
	pointLiteralRE     = regexp.MustCompile(`(?i)\bpoint\s+('(?:''|[^'])*')`)
	createIndexUsingRE = regexp.MustCompile(`(?i)(CREATE\s+(?:UNIQUE\s+)?INDEX\b.+?\bON\s+\S+)\s+USING\s+\S+\s*(\()`)
	ilikeRE            = regexp.MustCompile(`(?i)\bILIKE\b`)
	returningRE        = regexp.MustCompile(`(?i)\bRETURNING\b`)
	onlyRE             = regexp.MustCompile(`(?i)\bFROM\s+ONLY\s+`)
	eStringRE          = regexp.MustCompile(`(?i)\bE'`)
	trueRE             = regexp.MustCompile(`(?i)\bTRUE\b`)
	falseRE            = regexp.MustCompile(`(?i)\bFALSE\b`)
	ifNotExistsRE      = regexp.MustCompile(`(?i)\bIF\s+NOT\s+EXISTS\b`)
	ifExistsRE         = regexp.MustCompile(`(?i)\bIF\s+EXISTS\b`)
	deleteFromRE       = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+([A-Za-z0-9_\".]+)\s+([A-Za-z0-9_\"]+)\s+`)
	pgCatalogPrefixRE  = regexp.MustCompile(`(?i)\bpg_catalog\.([a-zA-Z0-9_]+)\b`)
	anonymousIndexRE   = regexp.MustCompile(`(?i)\bCREATE\s+(UNIQUE\s+)?INDEX\s+ON\s+([A-Za-z0-9_\".]+)\b`)
	circleColRE        = regexp.MustCompile(`(?i)\b(\w+)\s+circle\b`)
)

var idxCounter atomic.Int64


func Rewrite(sql string) string {
	out := rewriteParametersAndCasts(sql)
	out = serialRE.ReplaceAllString(out, "integer")
	out = boolRE.ReplaceAllString(out, "integer")
	out = nowRE.ReplaceAllString(out, "datetime('now')")
	out = quotedCharRE.ReplaceAllString(out, "::text")
	out = rewritePGCast(out)
	out = charLiteralRE.ReplaceAllString(out, "$1")
	out = pgLsnLiteralRE.ReplaceAllString(out, "$1")
	out = pointLiteralRE.ReplaceAllString(out, "$1")
	out = rewriteAnonymousIndex(out)
	out = createIndexUsingRE.ReplaceAllString(out, "$1 $2")
	out = ilikeRE.ReplaceAllString(out, "LIKE")
	out = rewriteRegexOps(out)
	out = rewriteSimilarTo(out)
	out = rewriteNullsOrder(out)
	out = rewriteGenerateSeries(out)
	out = rewritePosition(out)
	out = rewriteDeleteAlias(out)
	out = rewriteDistanceOperator(out)
	out = circleColRE.ReplaceAllStringFunc(out, func(m string) string {
		parts := strings.Fields(m)
		first := strings.ToLower(parts[0])
		if first == "as" || first == "cast" || first == "type" || first == "using" {
			return m
		}
		return parts[0] + " text CHECK (is_valid_circle(" + parts[0] + "))"
	})
	out = pgCatalogPrefixRE.ReplaceAllString(out, "$1")
	out = onlyRE.ReplaceAllString(out, "FROM ")
	out = eStringRE.ReplaceAllString(out, "'")
	return out
}

func rewriteAnonymousIndex(sql string) string {
	return anonymousIndexRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := anonymousIndexRE.FindStringSubmatch(m)
		uniqueVal := idxCounter.Add(1)
		uniqueName := fmt.Sprintf("idx_anon_%d", uniqueVal)

		uniqueStr := "CREATE "
		if strings.TrimSpace(sub[1]) != "" {
			uniqueStr += "UNIQUE "
		}
		uniqueStr += "INDEX " + uniqueName + " ON " + sub[2]
		return uniqueStr
	})
}

func rewriteDistanceOperator(sql string) string {
	idx := strings.Index(sql, "<->")
	if idx == -1 {
		return sql
	}

	// Parse left operand
	leftStart := idx
	for leftStart > 0 && (sql[leftStart-1] == ' ' || sql[leftStart-1] == '\t') {
		leftStart--
	}
	leftEnd := leftStart
	if leftStart > 0 && sql[leftStart-1] == '\'' {
		leftStart--
		for leftStart > 0 && sql[leftStart-1] != '\'' {
			leftStart--
		}
		if leftStart > 0 {
			leftStart--
		}
	} else if leftStart > 0 && sql[leftStart-1] == ')' {
		depth := 1
		leftStart--
		for leftStart > 0 && depth > 0 {
			leftStart--
			if sql[leftStart] == ')' {
				depth++
			} else if sql[leftStart] == '(' {
				depth--
			}
		}
		for leftStart > 0 && isIdentByte(sql[leftStart-1]) {
			leftStart--
		}
	} else {
		for leftStart > 0 && (isIdentByte(sql[leftStart-1]) || sql[leftStart-1] == '.') {
			leftStart--
		}
	}

	leftExpr := sql[leftStart:leftEnd]

	// Parse right operand
	rightStart := idx + 3
	for rightStart < len(sql) && (sql[rightStart] == ' ' || sql[rightStart] == '\t') {
		rightStart++
	}
	rightEnd := rightStart
	if rightStart < len(sql) && sql[rightStart] == '\'' {
		rightEnd++
		for rightEnd < len(sql) && sql[rightEnd] != '\'' {
			rightEnd++
		}
		if rightEnd < len(sql) {
			rightEnd++
		}
	} else if rightStart < len(sql) && isIdentByte(sql[rightStart]) {
		for rightEnd < len(sql) && (isIdentByte(sql[rightEnd]) || sql[rightEnd] == '.') {
			rightEnd++
		}
		if rightEnd < len(sql) && sql[rightEnd] == '(' {
			depth := 1
			rightEnd++
			for rightEnd < len(sql) && depth > 0 {
				if sql[rightEnd] == '(' {
					depth++
				} else if sql[rightEnd] == ')' {
					depth--
				}
				rightEnd++
			}
		}
	} else if rightStart < len(sql) && sql[rightStart] == '(' {
		depth := 1
		rightEnd++
		for rightEnd < len(sql) && depth > 0 {
			if sql[rightEnd] == '(' {
				depth++
			} else if sql[rightEnd] == ')' {
				depth--
			}
			rightEnd++
		}
	}

	rightExpr := sql[rightStart:rightEnd]

	rewritten := sql[:leftStart] + "dist_op(" + leftExpr + ", " + rightExpr + ")" + sql[rightEnd:]
	return rewriteDistanceOperator(rewritten)
}

func rewriteParametersAndCasts(sql string) string {
	var sb strings.Builder
	inQuote := false
	inDoubleQuote := false
	n := len(sql)
	for i := 0; i < n; i++ {
		ch := sql[i]
		if inQuote {
			sb.WriteByte(ch)
			if ch == '\'' {
				if i+1 < n && sql[i+1] == '\'' {
					sb.WriteByte('\'')
					i++
				} else {
					inQuote = false
				}
			}
			continue
		}
		if inDoubleQuote {
			sb.WriteByte(ch)
			if ch == '"' {
				if i+1 < n && sql[i+1] == '"' {
					sb.WriteByte('"')
					i++
				} else {
					inDoubleQuote = false
				}
			}
			continue
		}

		if ch == '\'' {
			inQuote = true
			sb.WriteByte(ch)
			continue
		}
		if ch == '"' {
			inDoubleQuote = true
			sb.WriteByte(ch)
			continue
		}

		if ch == '$' && i+1 < n && sql[i+1] >= '0' && sql[i+1] <= '9' {
			j := i + 1
			for j < n && sql[j] >= '0' && sql[j] <= '9' {
				j++
			}
			if j+1 < n && sql[j] == ':' && sql[j+1] == ':' {
				k := j + 2
				for k < n && (isIdentByte(sql[k]) || sql[k] == ' ') {
					k++
				}
				typeName := strings.TrimSpace(sql[j+2 : k])
				sb.WriteString("cast(? as ")
				sb.WriteString(mapPGType(typeName))
				sb.WriteString(")")
				i = k - 1
				continue
			} else {
				sb.WriteByte('?')
				i = j - 1
				continue
			}
		}

		sb.WriteByte(ch)
	}
	return sb.String()
}

func rewritePGCast(sql string) string {
	for {
		loc := castRE.FindStringIndex(sql)
		if loc == nil {
			break
		}
		typeName := strings.TrimSpace(castRE.FindStringSubmatch(sql[loc[0]:])[1])
		typeName, trailing := splitCastTypeTrailingSQL(typeName)
		typeName, aliasTrailing := splitCastTypeAlias(typeName)
		trailing = aliasTrailing + trailing
		typeName = strings.TrimSuffix(typeName, "[]")
		typeName = strings.TrimSpace(typeName)
		if idx := strings.Index(typeName, "("); idx >= 0 {
			typeName = typeName[:idx]
		}
		typeName = mapPGType(typeName)
		beforeCast := strings.TrimRightFunc(sql[:loc[0]], unicode.IsSpace)
		expr := extractCastExpr(beforeCast)
		prefix := beforeCast[:len(beforeCast)-len(expr)]
		suffix := trailing + sql[loc[1]:]
		if expr == "" {
			sql = prefix + suffix
		} else {
			sql = prefix + "CAST(" + expr + " AS " + typeName + ")" + suffix
		}
	}
	return sql
}

func splitCastTypeTrailingSQL(typeName string) (string, string) {
	lower := strings.ToLower(typeName)
	for _, keyword := range []string{" from ", " where ", " order ", " group ", " limit ", " union ", " intersect ", " except ", " and ", " or ", " is ", " in ", " as "} {
		if idx := strings.Index(lower, keyword); idx >= 0 {
			return strings.TrimSpace(typeName[:idx]), typeName[idx:]
		}
	}
	return typeName, ""
}

func splitCastTypeAlias(typeName string) (string, string) {
	fields := strings.Fields(typeName)
	if len(fields) != 2 {
		return typeName, ""
	}
	lower := strings.ToLower(typeName)
	switch lower {
	case "double precision", "character varying":
		return typeName, ""
	}
	if strings.Contains(fields[1], "(") || strings.HasSuffix(fields[0], "[]") {
		return typeName, ""
	}
	return fields[0], " " + fields[1]
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
	case "regclass", "regtype", "oid", "regproc", "regoper", "circle":
		return "text"
	case "oid8":
		return "integer"
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
	for i > 0 {
		b := sql[i-1]
		if isIdentByte(b) || b == '.' {
			i--
			continue
		}
		if (b == '-' || b == '+') && i > 1 && (sql[i-2] == 'e' || sql[i-2] == 'E') {
			i--
			continue
		}
		break
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
	startIdx := 0
	for {
		if startIdx >= len(result) {
			break
		}
		idx := indexOutsideQuotes(strings.ToUpper(result[startIdx:]), "POSITION")
		if idx < 0 {
			break
		}
		idx += startIdx

		// Check if it's part of an identifier
		isIdent := false
		if idx > 0 && isIdentByte(result[idx-1]) {
			isIdent = true
		}
		if idx+len("POSITION") < len(result) && isIdentByte(result[idx+len("POSITION")]) {
			isIdent = true
		}

		if isIdent {
			startIdx = idx + len("POSITION")
			continue
		}

		open := idx + len("POSITION")
		for open < len(result) && unicode.IsSpace(rune(result[open])) {
			open++
		}
		if open >= len(result) || result[open] != '(' {
			startIdx = idx + len("POSITION")
			continue
		}
		close := findMatchingParen(result, open)
		if close < 0 {
			startIdx = open + 1
			continue
		}
		body := result[open+1 : close]
		inStart, inEnd := topLevelInBounds(body)
		if inStart < 0 {
			rewrittenBody := rewritePosition(body)
			if rewrittenBody != body {
				result = result[:open+1] + rewrittenBody + result[close:]
				close = open + 1 + len(rewrittenBody)
			}
			startIdx = close + 1
			continue
		}
		needle := strings.TrimSpace(body[:inStart])
		haystack := strings.TrimSpace(body[inEnd:])
		needle = rewritePosition(needle)
		haystack = rewritePosition(haystack)

		replacement := "instr(" + haystack + ", " + needle + ")"
		result = result[:idx] + replacement + result[close+1:]
		startIdx = idx + len(replacement)
	}
	return result
}

func rewriteDeleteAlias(sql string) string {
	result := sql
	startIdx := 0
	for {
		if startIdx >= len(result) {
			break
		}
		loc := deleteFromRE.FindStringIndex(result[startIdx:])
		if loc == nil {
			break
		}
		matchStart := loc[0] + startIdx
		matchEnd := loc[1] + startIdx
		match := result[matchStart:matchEnd]

		sub := deleteFromRE.FindStringSubmatch(match)
		if len(sub) < 3 {
			startIdx = matchEnd
			continue
		}
		table := sub[1]
		alias := sub[2]
		lowerAlias := strings.ToLower(alias)
		if lowerAlias == "as" || lowerAlias == "where" || lowerAlias == "using" || lowerAlias == "returning" {
			startIdx = matchEnd
			continue
		}

		idxTable := strings.Index(strings.ToLower(match), strings.ToLower(table))
		if idxTable < 0 {
			startIdx = matchEnd
			continue
		}
		idxAlias := strings.Index(strings.ToLower(match[idxTable+len(table):]), strings.ToLower(alias))
		if idxAlias < 0 {
			startIdx = matchEnd
			continue
		}
		idxAlias += idxTable + len(table)

		rewrittenMatch := match[:idxAlias] + "AS " + match[idxAlias:]

		stmtEnd := strings.Index(result[matchEnd:], ";")
		var rest string
		var suffix string
		if stmtEnd < 0 {
			rest = result[matchEnd:]
			suffix = ""
		} else {
			rest = result[matchEnd : matchEnd+stmtEnd]
			suffix = result[matchEnd+stmtEnd:]
		}

		escapedTable := regexp.QuoteMeta(table)
		tableRefRE, err := regexp.Compile(`(?i)\b` + escapedTable + `\.`)
		if err == nil {
			rest = tableRefRE.ReplaceAllString(rest, alias+".")
		}

		result = result[:matchStart] + rewrittenMatch + rest + suffix
		startIdx = matchStart + len(rewrittenMatch) + len(rest)
	}
	return result
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
