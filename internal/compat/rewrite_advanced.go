package compat

import (
	"fmt"
	"regexp"
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
	startPos := 0
	for {
		idx := findRegexOpFrom(sql, op, startPos)
		if idx < 0 {
			break
		}

		lhsEnd := idx
		for lhsEnd > 0 && sql[lhsEnd-1] == ' ' {
			lhsEnd--
		}
		// Check what comes before the LHS: if it's a comma, open paren, =, comparison op, or start of SELECT list,
		// then ~ is a unary (bitwise NOT) operator, not a regex binary operator.
		lhsTrimmed := lhsEnd
		if lhsTrimmed > 0 {
			prevChar := sql[lhsTrimmed-1]
			if prevChar == ',' || prevChar == '(' || prevChar == '=' || prevChar == '<' || prevChar == '>' {
				startPos = idx + len(op)
				continue
			}
		}
		lhs := extractExprBefore(sql, lhsEnd)
		if lhs == "" || isSQLKeyword(lhs) {
			// Unary ~ (bitwise NOT) or keyword context: skip this occurrence
			startPos = idx + len(op)
			continue
		}
		lhsStart := lhsEnd - len(lhs)
		// If something precedes the LHS expression (comma/paren), it's unary
		if lhsStart > 0 {
			prevChar := sql[lhsStart-1]
			if prevChar == ',' || prevChar == '(' || prevChar == '=' {
				startPos = idx + len(op)
				continue
			}
		}

		rhsStart := idx + len(op)
		for rhsStart < len(sql) && sql[rhsStart] == ' ' {
			rhsStart++
		}
		if rhsStart >= len(sql) {
			break
		}

		// Extract RHS: either a string literal, identifier, or function call like CAST(...)
		var rhsEnd int
		var pattern string
		if sql[rhsStart] == '\'' {
			// String literal — extract pattern
			rhsEnd = rhsStart + 1
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
			pattern = sql[rhsStart+1 : rhsEnd-1]
		} else if isIdentByte(sql[rhsStart]) || sql[rhsStart] == '(' {
			// Identifier or parenthesized expression: use the whole RHS as expression
			rhsEnd = extractExprEnd(sql, rhsStart)
			if rhsEnd <= rhsStart {
				break
			}
			// RHS is an expression; use it directly in the function call
			rhsExpr := sql[rhsStart:rhsEnd]
			flags := "''"
			if icase {
				flags = "'i'"
			}
			var replacement string
			if negate {
				replacement = fmt.Sprintf("NOT minipsql_regexp_match(%s, %s, %s)", rhsExpr, lhs, flags)
			} else {
				replacement = fmt.Sprintf("minipsql_regexp_match(%s, %s, %s)", rhsExpr, lhs, flags)
			}
			sql = sql[:lhsStart] + replacement + sql[rhsEnd:]
			startPos = 0
			continue
		} else {
			startPos = idx + len(op)
			continue
		}

		flags := "''"
		if icase {
			flags = "'i'"
		}
		var replacement string
		if negate {
			replacement = fmt.Sprintf("NOT minipsql_regexp_match('%s', %s, %s)", pattern, lhs, flags)
		} else {
			replacement = fmt.Sprintf("minipsql_regexp_match('%s', %s, %s)", pattern, lhs, flags)
		}

		sql = sql[:lhsStart] + replacement + sql[rhsEnd:]
	}
	return sql
}

// extractExprEnd finds the end of an expression starting at pos.
func extractExprEnd(sql string, pos int) int {
	if pos >= len(sql) {
		return pos
	}
	if sql[pos] == '(' {
		depth := 1
		i := pos + 1
		inSingle := false
		for i < len(sql) {
			c := sql[i]
			if inSingle {
				if c == '\'' && i+1 < len(sql) && sql[i+1] == '\'' {
					i += 2
					continue
				}
				if c == '\'' {
					inSingle = false
				}
				i++
				continue
			}
			if c == '\'' {
				inSingle = true
			} else if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					return i + 1
				}
			}
			i++
		}
		return i
	}
	// Identifier (possibly with schema and function call)
	i := pos
	for i < len(sql) && (isIdentByte(sql[i]) || sql[i] == '.') {
		i++
	}
	// Check for function call
	if i < len(sql) && sql[i] == '(' {
		return extractExprEnd(sql, i)
	}
	return i
}

// isSQLKeyword returns true if s is a SQL keyword that cannot be an expression LHS for ~.
func isSQLKeyword(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "SELECT", "WHERE", "FROM", "AND", "OR", "NOT", "ON", "SET",
		"THEN", "ELSE", "CASE", "WHEN", "END", "IN", "AS", "BY",
		"HAVING", "JOIN", "LEFT", "RIGHT", "INNER", "OUTER", "CROSS",
		"UNION", "INTERSECT", "EXCEPT", "ALL", "DISTINCT", "LIMIT",
		"ORDER", "GROUP", "INSERT", "UPDATE", "DELETE", "INTO", "VALUES":
		return true
	}
	return false
}

func findRegexOp(sql string, op string) int {
	return findRegexOpFrom(sql, op, 0)
}

func findRegexOpFrom(sql string, op string, startPos int) int {
	inSingle := false
	inDouble := false
	escaped := false
	for i := startPos; i < len(sql); i++ {
		if sql[i] == '"' && !inSingle {
			if inDouble && i+1 < len(sql) && sql[i+1] == '"' {
				i++
				continue
			}
			inDouble = !inDouble
			continue
		}
		if inDouble {
			continue
		}
		if sql[i] == '\'' && !escaped {
			if inSingle && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			inSingle = !inSingle
			continue
		}
		if inSingle {
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
	lower := strings.ToLower(sql)
	for {
		idx := indexOutsideQuotes(lower, "similar to")
		if idx == -1 {
			break
		}
		
		exprEnd := idx
		for exprEnd > 0 && sql[exprEnd-1] == ' ' {
			exprEnd--
		}
		lhs := extractExprBefore(sql, exprEnd)
		lhsStart := exprEnd - len(lhs)
		
		patStart := idx + len("similar to")
		escapeIdx := -1
		endIdx := len(sql)
		
		depth := 0
		inSingle := false
		inDouble := false
		
		for j := patStart; j < len(sql); j++ {
			if inSingle {
				if sql[j] == '\'' {
					if j+1 < len(sql) && sql[j+1] == '\'' {
						j++
						continue
					}
					inSingle = false
				}
				continue
			}
			if inDouble {
				if sql[j] == '"' {
					if j+1 < len(sql) && sql[j+1] == '"' {
						j++
						continue
					}
					inDouble = false
				}
				continue
			}
			
			switch sql[j] {
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case '(':
				depth++
			case ')':
				if depth == 0 {
					endIdx = j
					goto done
				}
				depth--
			case ',', ';':
				if depth == 0 {
					endIdx = j
					goto done
				}
			}
			
			if depth == 0 && j+6 < len(sql) && strings.ToLower(sql[j:j+6]) == "escape" {
				if (j == 0 || isSpaceOrOp(sql[j-1])) && (j+6 == len(sql) || isSpaceOrOp(sql[j+6])) {
					escapeIdx = j
				}
			}
			
			if depth == 0 {
				for _, kw := range []string{"and", "or", "where", "order", "group", "limit", "union", "intersect", "except", "as"} {
					kwLen := len(kw)
					if j+kwLen <= len(sql) && strings.ToLower(sql[j:j+kwLen]) == kw {
						if (j == 0 || isSpaceOrOp(sql[j-1])) && (j+kwLen == len(sql) || isSpaceOrOp(sql[j+kwLen])) {
							endIdx = j
							goto done
						}
					}
				}
			}
		}
	done:
		
		var patExpr, escExpr string
		var actualEnd int
		if escapeIdx != -1 && escapeIdx < endIdx {
			patExpr = strings.TrimSpace(sql[patStart:escapeIdx])
			escExpr = strings.TrimSpace(sql[escapeIdx+len("escape"):endIdx])
			actualEnd = endIdx
		} else {
			patExpr = strings.TrimSpace(sql[patStart:endIdx])
			escExpr = "'\\'"
			actualEnd = endIdx
		}
		
		rewritten := "minipsql_similar_to(" + lhs + ", " + patExpr + ", " + escExpr + ")"
		sql = sql[:lhsStart] + rewritten + sql[actualEnd:]
		lower = strings.ToLower(sql)
	}
	return sql
}

func isSpaceOrOp(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '(' || c == ')' || c == ',' || c == ';' || c == '=' || c == '<' || c == '>' || c == '!'
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
		// Use last occurrence so nested generate_series(1, generate_series(1,3)) is
		// rewritten from the inside out — innermost first, then outer.
		idx := lastIndexOutsideQuotes(strings.ToUpper(result), "GENERATE_SERIES")
		if idx < 0 {
			break
		}

		fromIdx := -1
		prefix := strings.ToUpper(result[:idx])

		// Skip generate_series that appears after ORDER BY, HAVING, GROUP BY, or WHERE
		// at the top level — injecting a CTE there would break the SQL structure.
		// Detect this by checking if the last clause-start keyword before idx is one of these.
		skipClauseStart := -1
		for _, kw := range []string{"ORDER BY", "HAVING", "GROUP BY", "WHERE"} {
			kwIdx := strings.LastIndex(prefix, kw)
			if kwIdx > skipClauseStart {
				skipClauseStart = kwIdx
			}
		}
		lastFromOrSelect := strings.LastIndex(prefix, "FROM")
		if lastSelectIdx := strings.LastIndex(prefix, "SELECT"); lastSelectIdx > lastFromOrSelect {
			lastFromOrSelect = lastSelectIdx
		}
		if skipClauseStart > lastFromOrSelect {
			// generate_series is in ORDER BY/WHERE/HAVING/GROUP BY context — skip this occurrence
			// by replacing with a marker that won't re-match "GENERATE_SERIES".
			result = result[:idx] + "_gs_skipped_" + result[idx+len("GENERATE_SERIES"):]
			continue
		}

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
		start := stripSubqueryAlias(strings.TrimSpace(argParts[0]))
		stop := ""
		if len(argParts) > 1 {
			stop = stripSubqueryAlias(strings.TrimSpace(argParts[1]))
		}
		step := "1"
		if len(argParts) > 2 {
			step = stripSubqueryAlias(strings.TrimSpace(argParts[2]))
		}

		// Parse optional alias forms: AS alias(col), alias(col), or alias.
		rest := result[parenEnd:]
		alias := "_gs_alias"
		colName := "value"
		trimmed := strings.TrimSpace(rest)
		leadingSpace := len(rest) - len(trimmed)
		parsedAlias, parsedCol, consumedTrimmed := parseGenerateSeriesAlias(trimmed)
		consumed := 0
		if parsedAlias != "" {
			alias = parsedAlias
			if parsedCol == "" {
				colName = parsedAlias
			} else {
				colName = parsedCol
			}
			consumed = leadingSpace + consumedTrimmed
		}

		cmpOp := "<="
		stepTrimmed := strings.TrimSpace(step)
		if strings.HasPrefix(stepTrimmed, "-") {
			cmpOp = ">="
		}

		// step = 0 would cause infinite loop; emit error instead
		zeroStep := stepTrimmed == "0" || stepTrimmed == "+0" || stepTrimmed == "-0"
		// INTERVAL '0 ...' also counts as zero step
		if !zeroStep && strings.Contains(strings.ToUpper(stepTrimmed), "INTERVAL") {
			inner := regexp.MustCompile(`INTERVAL\s+'([^']*)'`).FindStringSubmatch(strings.ToUpper(stepTrimmed))
			if len(inner) > 1 {
				// check if all numeric parts are zero
				fields := regexp.MustCompile(`[\d.]+`).FindAllString(inner[1], -1)
				allZero := len(fields) > 0
				for _, f := range fields {
					if f != "0" && f != "0.0" {
						allZero = false
						break
					}
				}
				if allZero {
					zeroStep = true
				}
			}
		}

		// infinity start/stop with timestamp generate_series would loop forever in SQLite
		stopLower := strings.ToLower(strings.TrimSpace(stop))
		startLower := strings.ToLower(strings.TrimSpace(start))
		hasInfinity := strings.Contains(stopLower, "infinity") || strings.Contains(startLower, "infinity")
		// INTERVAL step cannot be used natively in SQLite recursive CTEs (date arithmetic differs)
		hasInterval := strings.Contains(strings.ToUpper(stepTrimmed), "INTERVAL")

		cteID := idxCounter.Add(1)
		cteName := fmt.Sprintf("_gs_cte_%d", cteID)
		var cte string
		if zeroStep {
			cte = fmt.Sprintf(
				"(SELECT minipsql_error('step size cannot equal zero', 'XX000', '')) AS %s",
				alias,
			)
		} else if hasInfinity || hasInterval {
			// Cannot iterate over infinite range or INTERVAL step in SQLite recursive CTE; return empty.
			cte = fmt.Sprintf("(SELECT NULL AS %s WHERE 1=0) AS %s", colName, alias)
		} else {
			cte = fmt.Sprintf(
				"(WITH RECURSIVE %s(%s) AS (SELECT %s UNION ALL SELECT %s + (%s) FROM %s WHERE %s + (%s) %s %s) SELECT %s FROM %s) AS %s",
				cteName, colName, start, colName, step, cteName, colName, step, cmpOp, stop, colName, cteName, alias,
			)
		}

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
	// Restore any skipped generate_series occurrences (in ORDER BY / WHERE / etc.)
	result = strings.ReplaceAll(result, "_gs_skipped_", "generate_series")
	return result
}

// stripSubqueryAlias removes a trailing " AS alias" from a scalar subquery like
// "(WITH RECURSIVE ...) AS _gs_alias" so it can be used in an expression context.
func stripSubqueryAlias(s string) string {
	if !strings.HasPrefix(s, "(") {
		return s
	}
	// Find the matching closing paren
	depth := 0
	closePos := -1
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closePos = i
			}
		}
		if closePos >= 0 {
			break
		}
	}
	if closePos < 0 || closePos == len(s)-1 {
		return s // no trailing content
	}
	tail := strings.TrimSpace(s[closePos+1:])
	if strings.HasPrefix(strings.ToUpper(tail), "AS ") {
		return s[:closePos+1] // strip " AS alias"
	}
	return s
}

func parseGenerateSeriesAlias(trimmed string) (alias string, colName string, consumed int) {
	if trimmed == "" {
		return "", "", 0
	}
	pos := 0
	if len(trimmed) >= 2 && strings.EqualFold(trimmed[:2], "AS") && (len(trimmed) == 2 || trimmed[2] == ' ') {
		pos = 2
		for pos < len(trimmed) && trimmed[pos] == ' ' {
			pos++
		}
	}
	setOps := []string{"WHERE", "ORDER", "GROUP", "LIMIT", "UNION", "EXCEPT", "INTERSECT", "HAVING", "ON", "JOIN", "CROSS", "INNER", "LEFT", "RIGHT", "FULL", "NATURAL"}
	checkKeyword := func(s string) bool {
		for _, kw := range setOps {
			if startsWithKeyword(s, kw) {
				return true
			}
		}
		return false
	}
	if checkKeyword(trimmed[pos:]) {
		return "", "", 0
	}

	aliasStart := pos
	for pos < len(trimmed) && (isIdentByte(trimmed[pos]) || trimmed[pos] == '_') {
		pos++
	}
	if pos == aliasStart {
		return "", "", 0
	}
	alias = trimmed[aliasStart:pos]
	if checkKeyword(alias) {
		return "", "", 0
	}

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
	return alias, colName, pos
}

func startsWithKeyword(s string, keyword string) bool {
	if len(s) < len(keyword) || !strings.EqualFold(s[:len(keyword)], keyword) {
		return false
	}
	return len(s) == len(keyword) || s[len(keyword)] == ' '
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

// rewriteDistinctOn converts SELECT DISTINCT ON (exprs) ... to a ROW_NUMBER() window query.
// PG: SELECT DISTINCT ON (e1, e2) cols FROM t ORDER BY e1, e2, e3
// → SELECT cols FROM (SELECT *, ROW_NUMBER() OVER (PARTITION BY e1,e2 ORDER BY e1,e2,e3) AS __dn FROM t) WHERE __dn=1 ORDER BY e1,e2,e3
func rewriteDistinctOn(sql string) string {
	// Find "SELECT DISTINCT ON (" pattern (case-insensitive, outside quotes)
	idx := findDistinctOnStart(sql)
	if idx < 0 {
		return sql
	}

	// prelude = everything before "SELECT"
	prelude := sql[:idx]

	// after "SELECT " is "DISTINCT ON ..."
	body := sql[idx+len("SELECT "):]
	bodyUpper := strings.ToUpper(body)

	if !strings.HasPrefix(bodyUpper, "DISTINCT ON ") && !strings.HasPrefix(bodyUpper, "DISTINCT ON(") {
		return sql
	}

	// Skip "DISTINCT ON" and find opening paren
	afterDistinctOn := len("DISTINCT ON")
	for afterDistinctOn < len(body) && body[afterDistinctOn] == ' ' {
		afterDistinctOn++
	}
	if afterDistinctOn >= len(body) || body[afterDistinctOn] != '(' {
		return sql
	}

	// Extract DISTINCT ON exprs
	parenStart := afterDistinctOn
	parenEnd := findMatchingParen(body, parenStart)
	if parenEnd < 0 {
		return sql
	}
	distinctExprs := body[parenStart+1 : parenEnd]

	// Rest after DISTINCT ON (...)
	rest := strings.TrimSpace(body[parenEnd+1:])

	// Find FROM keyword (outside parens/quotes) to split col list from FROM clause
	fromIdx := findKeywordOutsideParens(rest, "FROM")
	if fromIdx < 0 {
		return sql
	}
	colList := strings.TrimSpace(rest[:fromIdx])
	fromClause := rest[fromIdx+len("FROM"):]

	// Find ORDER BY in what follows FROM clause
	// We need to find ORDER BY that belongs to this SELECT (not a subquery)
	orderByIdx := findTopLevelKeyword(fromClause, "ORDER BY")
	var fromBody, orderByClause string
	if orderByIdx >= 0 {
		fromBody = strings.TrimSpace(fromClause[:orderByIdx])
		orderByClause = strings.TrimSpace(fromClause[orderByIdx+len("ORDER BY"):])
	} else {
		fromBody = strings.TrimSpace(fromClause)
		orderByClause = ""
	}

	// Build window ORDER BY: prefer explicit ORDER BY, else use DISTINCT exprs
	var windowOrder string
	if orderByClause != "" {
		windowOrder = orderByClause
	} else {
		windowOrder = distinctExprs
	}

	// Strip trailing semicolon from orderByClause for LIMIT detection
	limitIdx := findTopLevelKeyword(orderByClause, "LIMIT")
	var limitClause, orderByNoLimit string
	if limitIdx >= 0 {
		orderByNoLimit = strings.TrimSpace(orderByClause[:limitIdx])
		limitClause = " LIMIT " + strings.TrimSpace(orderByClause[limitIdx+len("LIMIT"):])
		windowOrder = orderByNoLimit
	} else {
		orderByNoLimit = orderByClause
		limitClause = ""
	}

	// Outer trailing ORDER BY
	var outerOrderBy string
	if orderByNoLimit != "" {
		outerOrderBy = " ORDER BY " + orderByNoLimit
	}

	// Build transformation
	// outer col list: if it's just "*" we keep it since sqlite will return all cols incl __dn
	// but we need to avoid exposing __dn. Use colList as-is.
	transformed := prelude + "SELECT " + colList +
		" FROM (SELECT *, ROW_NUMBER() OVER (PARTITION BY " + distinctExprs +
		" ORDER BY " + windowOrder + ") AS __dn FROM " + fromBody +
		") AS __dn_sub WHERE __dn = 1" + outerOrderBy + limitClause

	return transformed
}

// findDistinctOnStart returns the index of "SELECT " before a "DISTINCT ON" pattern.
// Returns -1 if not found or if inside quotes/parens.
func findDistinctOnStart(sql string) int {
	inQuote := false
	depth := 0
	i := 0
	for i < len(sql) {
		c := sql[i]
		if c == '\'' && !inQuote {
			inQuote = true
			i++
			continue
		}
		if c == '\'' && inQuote {
			if i+1 < len(sql) && sql[i+1] == '\'' {
				i += 2
				continue
			}
			inQuote = false
			i++
			continue
		}
		if inQuote {
			i++
			continue
		}
		if c == '(' {
			depth++
			i++
			continue
		}
		if c == ')' {
			depth--
			i++
			continue
		}
		if depth == 0 {
			upper := strings.ToUpper(sql[i:])
			if strings.HasPrefix(upper, "SELECT ") || strings.HasPrefix(upper, "SELECT\t") || strings.HasPrefix(upper, "SELECT\n") {
				// Check if "DISTINCT ON" follows (after optional whitespace)
				selectEnd := i + 7
				for selectEnd < len(sql) && (sql[selectEnd] == ' ' || sql[selectEnd] == '\t' || sql[selectEnd] == '\n') {
					selectEnd++
				}
				rem := strings.ToUpper(sql[selectEnd:])
				if strings.HasPrefix(rem, "DISTINCT ON ") || strings.HasPrefix(rem, "DISTINCT ON(") {
					return i
				}
			}
		}
		i++
	}
	return -1
}

// findKeywordOutsideParens finds the first occurrence of a keyword at depth=0 (outside parens/quotes).
func findKeywordOutsideParens(s string, keyword string) int {
	upper := strings.ToUpper(keyword)
	inQuote := false
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' && !inQuote {
			inQuote = true
			continue
		}
		if c == '\'' && inQuote {
			if i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			inQuote = false
			continue
		}
		if inQuote {
			continue
		}
		if c == '(' {
			depth++
			continue
		}
		if c == ')' {
			depth--
			continue
		}
		if depth == 0 && i+len(keyword) <= len(s) {
			chunk := strings.ToUpper(s[i : i+len(keyword)])
			if chunk == upper {
				// Make sure it's a word boundary
				before := i == 0 || !isIdentByte(s[i-1])
				after := i+len(keyword) >= len(s) || !isIdentByte(s[i+len(keyword)])
				if before && after {
					return i
				}
			}
		}
	}
	return -1
}

// findTopLevelKeyword is like findKeywordOutsideParens but also handles double-word keywords like "ORDER BY".
func findTopLevelKeyword(s string, keyword string) int {
	return findKeywordOutsideParens(s, keyword)
}

// findCaseInsensitive finds substr in s case-insensitively.
func findCaseInsensitive(s string, substr string) int {
	upper := strings.ToUpper(s)
	upperSub := strings.ToUpper(substr)
	return strings.Index(upper, upperSub)
}

// partitionSRFs maps SRF names to their stub SELECT columns (always empty, no partitioning support).
var partitionSRFs = map[string]string{
	"pg_partition_tree":      "SELECT NULL AS relid, NULL AS parentrelid, NULL AS isleaf, NULL AS level",
	"pg_partition_ancestors": "SELECT NULL AS relid",
}

// rewritePartitionSRFs rewrites FROM pg_partition_tree(arg) and FROM pg_partition_ancestors(arg)
// to inline subqueries that return empty result sets (minipsql has no partitioning support).
func rewritePartitionSRFs(sql string) string {
	lower := strings.ToLower(sql)
	for fnName, cols := range partitionSRFs {
		for {
			idx := strings.Index(lower, fnName)
			if idx == -1 {
				break
			}
			// Check word boundary before
			if idx > 0 && isIdentByte(sql[idx-1]) {
				lower = lower[:idx] + strings.ToUpper(fnName) + lower[idx+len(fnName):]
				continue
			}
			// Skip whitespace after function name
			j := idx + len(fnName)
			for j < len(sql) && (sql[j] == ' ' || sql[j] == '\t') {
				j++
			}
			if j >= len(sql) || sql[j] != '(' {
				lower = lower[:idx] + strings.ToUpper(fnName) + lower[idx+len(fnName):]
				continue
			}
			// Find matching close paren
			end := findMatchingParen(sql, j)
			if end < 0 {
				break
			}
			// Check for AS alias after the closing paren
			after := end + 1
			for after < len(sql) && (sql[after] == ' ' || sql[after] == '\t') {
				after++
			}
			alias := fnName
			aliasEnd := after
			if after < len(sql) && strings.EqualFold(sql[after:min2(after+3, len(sql))], "as ") {
				aliasEnd = after + 3
				for aliasEnd < len(sql) && (sql[aliasEnd] == ' ' || sql[aliasEnd] == '\t') {
					aliasEnd++
				}
				aliasStart := aliasEnd
				for aliasEnd < len(sql) && isAlphaNumeric(sql[aliasEnd]) {
					aliasEnd++
				}
				if aliasEnd > aliasStart {
					alias = sql[aliasStart:aliasEnd]
				}
			} else if after < len(sql) && isAlphaNumeric(sql[after]) {
				aliasStart := after
				for aliasEnd < len(sql) && isAlphaNumeric(sql[aliasEnd]) {
					aliasEnd++
				}
				if aliasEnd > aliasStart {
					alias = sql[aliasStart:aliasEnd]
				} else {
					aliasEnd = after
				}
			}
			replacement := fmt.Sprintf("(%s WHERE 0) AS %s", cols, alias)
			sql = sql[:idx] + replacement + sql[aliasEnd:]
			lower = strings.ToLower(sql)
		}
	}
	return sql
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
