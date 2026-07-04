package compat

import (
	"fmt"
	"strings"
)

func rewriteRegexOps(sql string) string {
	sql = rewriteRegexOp(sql, "!~*", true, true)
	sql = rewriteRegexOp(sql, "~*", false, true)
	sql = rewriteRegexOp(sql, "!~", true, false)
	sql = rewriteRegexOp(sql, "~", false, false)
	return sql
}

func rewriteRegexOp(sql string, op string, negate bool, icase bool) string {
	for {
		idx := findRegexOp(sql, op)
		if idx < 0 {
			break
		}

		lhsEnd := idx
		for lhsEnd > 0 && sql[lhsEnd-1] == ' ' {
			lhsEnd--
		}
		lhs := extractExprBefore(sql, lhsEnd)
		lhsStart := lhsEnd - len(lhs)

		rhsStart := idx + len(op)
		for rhsStart < len(sql) && sql[rhsStart] == ' ' {
			rhsStart++
		}
		if rhsStart >= len(sql) || sql[rhsStart] != '\'' {
			break
		}
		rhsEnd := rhsStart + 1
		for rhsEnd < len(sql) {
			if sql[rhsEnd] == '\'' {
				if rhsEnd+1 < len(sql) && sql[rhsEnd+1] == '\'' {
					rhsEnd += 2
					continue
				}
				break
			}
			if sql[rhsEnd] == '\\' {
				rhsEnd++
			}
			rhsEnd++
		}
		if rhsEnd >= len(sql) {
			break
		}
		rhsEnd++
		pattern := sql[rhsStart+1 : rhsEnd-1]

		flags := "''"
		if icase {
			flags = "'i'"
		}
		var replacement string
		if negate {
			replacement = fmt.Sprintf("NOT regexp_match('%s', %s, %s)", pattern, lhs, flags)
		} else {
			replacement = fmt.Sprintf("regexp_match('%s', %s, %s)", pattern, lhs, flags)
		}

		sql = sql[:lhsStart] + replacement + sql[rhsEnd:]
	}
	return sql
}

func findRegexOp(sql string, op string) int {
	inQuote := false
	escaped := false
	for i := 0; i < len(sql); i++ {
		if sql[i] == '\'' && !escaped {
			if inQuote && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			escaped = sql[i] == '\\' && !escaped
			continue
		}
		if i+len(op) > len(sql) {
			break
		}
		if sql[i:i+len(op)] == op {
			if op == "~" {
				if i > 0 && sql[i-1] == '!' {
					continue
				}
				if i+1 < len(sql) && sql[i+1] == '*' {
					continue
				}
			}
			if op == "!~" {
				if i+2 < len(sql) && sql[i+2] == '*' {
					continue
				}
			}
			return i
		}
	}
	return -1
}

func extractExprBefore(sql string, end int) string {
	i := end - 1
	if i < 0 {
		return ""
	}

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
		return sql[i:end]
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
		return sql[i:end]
	}

	for i > 0 && (isIdentByte(sql[i-1]) || sql[i-1] == '.') {
		i--
	}
	return sql[i:end]
}

func rewriteSimilarTo(sql string) string {
	result := sql
	upper := strings.ToUpper(sql)
	for {
		idx := indexOutsideQuotes(upper, "SIMILAR TO")
		if idx < 0 {
			break
		}

		exprEnd := idx
		for exprEnd > 0 && result[exprEnd-1] == ' ' {
			exprEnd--
		}
		lhs := extractExprBefore(result, exprEnd)
		lhsStart := exprEnd - len(lhs)

		patStart := idx + len("SIMILAR TO")
		for patStart < len(result) && result[patStart] == ' ' {
			patStart++
		}
		if patStart >= len(result) || result[patStart] != '\'' {
			break
		}
		patEnd := patStart + 1
		for patEnd < len(result) {
			if result[patEnd] == '\'' {
				if patEnd+1 < len(result) && result[patEnd+1] == '\'' {
					patEnd += 2
					continue
				}
				break
			}
			if result[patEnd] == '\\' {
				patEnd++
			}
			patEnd++
		}
		if patEnd >= len(result) {
			break
		}
		patEnd++
		pattern := result[patStart+1 : patEnd-1]

		regexPat := similarToRegex(pattern)
		replacement := fmt.Sprintf("regexp_match('%s', %s, '')", regexPat, lhs)

		result = result[:lhsStart] + replacement + result[patEnd:]
		upper = strings.ToUpper(result)
	}
	return result
}

func similarToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteString(".")
		case '(':
			b.WriteString("(")
		case ')':
			b.WriteString(")")
		case '|':
			b.WriteString("|")
		case '.', '^', '$', '+', '?', '{', '}', '[', ']', '\\', '*':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		default:
			b.WriteByte(pattern[i])
		}
	}
	b.WriteString("$")
	return b.String()
}

func rewriteNullsOrder(sql string) string {
	upper := strings.ToUpper(sql)
	orderIdx := lastIndexOutsideQuotes(upper, "ORDER BY")
	if orderIdx < 0 {
		return sql
	}

	prefix := sql[:orderIdx]
	orderClause := sql[orderIdx:]

	if !containsOutsideQuotes(strings.ToUpper(orderClause), "NULLS") {
		return sql
	}

	rewritten := rewriteNullsInOrderBy(orderClause)
	return prefix + rewritten
}

func rewriteNullsInOrderBy(orderClause string) string {
	afterOrderBy := orderClause[len("ORDER BY"):]
	items := splitOrderByItems(afterOrderBy)
	var rewritten []string
	for _, item := range items {
		rewritten = append(rewritten, rewriteSingleOrderItem(strings.TrimSpace(item)))
	}
	return "ORDER BY " + strings.Join(rewritten, ", ")
}

func splitOrderByItems(s string) []string {
	var items []string
	depth := 0
	inQuote := false
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			if inQuote && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
		} else if s[i] == ',' && depth == 0 {
			items = append(items, s[start:i])
			start = i + 1
		}
	}
	items = append(items, s[start:])
	return items
}

func rewriteSingleOrderItem(item string) string {
	trimmedItem := strings.TrimSpace(item)
	upper := strings.ToUpper(trimmedItem)

	var nullsPos string
	var cutIdx int
	if strings.HasSuffix(upper, " NULLS FIRST") {
		nullsPos = "FIRST"
		cutIdx = len(trimmedItem) - len(" NULLS FIRST")
	} else if strings.HasSuffix(upper, " NULLS LAST") {
		nullsPos = "LAST"
		cutIdx = len(trimmedItem) - len(" NULLS LAST")
	} else {
		return item
	}

	exprAndDir := strings.TrimSpace(trimmedItem[:cutIdx])

	upperED := strings.ToUpper(exprAndDir)
	direction := ""
	expr := exprAndDir
	if strings.HasSuffix(upperED, " ASC") {
		direction = "ASC"
		expr = strings.TrimSpace(exprAndDir[:len(exprAndDir)-4])
	} else if strings.HasSuffix(upperED, " DESC") {
		direction = "DESC"
		expr = strings.TrimSpace(exprAndDir[:len(exprAndDir)-5])
	}

	var nullVal, nonNullVal string
	if nullsPos == "FIRST" {
		nullVal = "0"
		nonNullVal = "1"
	} else {
		nullVal = "1"
		nonNullVal = "0"
	}

	nullSort := fmt.Sprintf("CASE WHEN %s IS NULL THEN %s ELSE %s END", expr, nullVal, nonNullVal)
	if direction != "" {
		return nullSort + ", " + expr + " " + direction
	}
	return nullSort + ", " + expr
}

func rewriteGenerateSeries(sql string) string {
	upper := strings.ToUpper(sql)
	if !strings.Contains(upper, "GENERATE_SERIES") {
		return sql
	}

	result := sql
	for {
		idx := indexOutsideQuotes(strings.ToUpper(result), "GENERATE_SERIES")
		if idx < 0 {
			break
		}

		fromIdx := -1
		prefix := strings.ToUpper(result[:idx])
		candidate := strings.LastIndex(prefix, "FROM")
		if candidate >= 0 {
			between := strings.TrimSpace(result[candidate+4 : idx])
			if between == "" {
				fromIdx = candidate
			}
		}
		if fromIdx < 0 {
			fromIdx = idx
		}

		parenStart := strings.Index(result[idx:], "(")
		if parenStart < 0 {
			break
		}
		parenStart += idx

		depth := 1
		parenEnd := parenStart + 1
		for parenEnd < len(result) && depth > 0 {
			if result[parenEnd] == '(' {
				depth++
			} else if result[parenEnd] == ')' {
				depth--
			}
			parenEnd++
		}

		argsStr := result[parenStart+1 : parenEnd-1]
		argParts := splitOrderByItems(argsStr)
		start := strings.TrimSpace(argParts[0])
		stop := ""
		if len(argParts) > 1 {
			stop = strings.TrimSpace(argParts[1])
		}
		step := "1"
		if len(argParts) > 2 {
			step = strings.TrimSpace(argParts[2])
		}

		// Parse optional AS alias(col)
		rest := result[parenEnd:]
		alias := "_gs_alias"
		colName := "value"
		consumed := 0
		trimmed := strings.TrimSpace(rest)
		leadingSpace := len(rest) - len(trimmed)
		if len(trimmed) >= 2 && strings.ToUpper(trimmed[:2]) == "AS" {
			pos := 2
			for pos < len(trimmed) && trimmed[pos] == ' ' {
				pos++
			}
			aliasStart := pos
			for pos < len(trimmed) && (isIdentByte(trimmed[pos]) || trimmed[pos] == '_') {
				pos++
			}
			if pos > aliasStart {
				alias = trimmed[aliasStart:pos]
				for pos < len(trimmed) && trimmed[pos] == ' ' {
					pos++
				}
				if pos < len(trimmed) && trimmed[pos] == '(' {
					closeP := strings.Index(trimmed[pos:], ")")
					if closeP > 0 {
						colName = strings.TrimSpace(trimmed[pos+1 : pos+closeP])
						pos += closeP + 1
					}
				}
				consumed = leadingSpace + pos
			}
		}

		cmpOp := "<="
		if strings.HasPrefix(strings.TrimSpace(step), "-") {
			cmpOp = ">="
		}

		cte := fmt.Sprintf(
			"(WITH RECURSIVE _gs_cte(%s) AS (SELECT %s UNION ALL SELECT %s + (%s) FROM _gs_cte WHERE %s + (%s) %s %s) SELECT %s FROM _gs_cte) AS %s",
			colName, start, colName, step, colName, step, cmpOp, stop, colName, alias,
		)

		var afterPart string
		if consumed > 0 {
			afterPart = rest[consumed:]
		} else {
			afterPart = rest
		}

		if fromIdx == idx {
			result = result[:fromIdx] + cte + " " + afterPart
		} else {
			result = result[:fromIdx] + "FROM " + cte + " " + afterPart
		}
	}
	return result
}

func indexOutsideQuotes(sql string, substr string) int {
	inQuote := false
	escaped := false
	for i := 0; i <= len(sql)-len(substr); i++ {
		if sql[i] == '\'' && !escaped {
			if inQuote && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			escaped = sql[i] == '\\' && !escaped
			continue
		}
		if sql[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func lastIndexOutsideQuotes(sql string, substr string) int {
	inQuote := false
	escaped := false
	last := -1
	for i := 0; i <= len(sql)-len(substr); i++ {
		if sql[i] == '\'' && !escaped {
			if inQuote && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			escaped = sql[i] == '\\' && !escaped
			continue
		}
		if sql[i:i+len(substr)] == substr {
			last = i
		}
	}
	return last
}

func containsOutsideQuotes(sql string, substr string) bool {
	return indexOutsideQuotes(sql, substr) >= 0
}
