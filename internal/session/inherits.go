package session

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
)

var inheritsRE = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+([A-Za-z0-9_\".\s]+)\s*\(([\s\S]*?)\)\s*INHERITS\s*\(([^)]+)\)`)

func (s *Session) rewriteInherits(ctx context.Context, sql string) (string, error) {
	loc := inheritsRE.FindStringSubmatchIndex(sql)
	if loc == nil {
		return sql, nil
	}

	match := sql[loc[0]:loc[1]]
	sub := inheritsRE.FindStringSubmatch(match)
	tableName := strings.TrimSpace(sub[1])
	childColsStr := sub[2]
	parentsStr := sub[3]

	parents := strings.Split(parentsStr, ",")
	var newCols []string
	seenCols := make(map[string]bool)

	// Fetch columns of all parent tables
	for _, parent := range parents {
		parent = strings.TrimSpace(parent)
		parentClean := strings.Trim(parent, `"'`)
		res, err := s.Backend.Query(ctx, fmt.Sprintf("PRAGMA table_info(%q)", parentClean), nil)
		if err != nil {
			return "", err
		}
		for _, row := range res.Rows {
			if len(row) < 3 {
				continue
			}
			cName := valStr(row[1])
			cType := valStr(row[2])
			lowerName := strings.ToLower(cName)
			if !seenCols[lowerName] {
				seenCols[lowerName] = true
				newCols = append(newCols, fmt.Sprintf("%s %s", cName, cType))
			}
		}
	}

	// Append child's own columns
	for _, colDef := range splitSQLColumns(childColsStr) {
		colDef = strings.TrimSpace(colDef)
		if colDef == "" {
			continue
		}
		parts := strings.Fields(colDef)
		if len(parts) > 0 {
			colName := strings.Trim(parts[0], `"'`)
			lowerName := strings.ToLower(colName)
			if !seenCols[lowerName] {
				seenCols[lowerName] = true
				newCols = append(newCols, colDef)
			}
		}
	}

	// Construct the rewritten CREATE TABLE statement
	rewritten := fmt.Sprintf("CREATE TABLE %s (%s)", tableName, strings.Join(newCols, ", "))
	
	// Replace in the original SQL
	sql = sql[:loc[0]] + rewritten + sql[loc[1]:]

	// Recursively handle any other CREATE TABLE ... INHERITS in the same SQL string
	return s.rewriteInherits(ctx, sql)
}

func valStr(v backend.Value) string {
	if v.Value == nil {
		return ""
	}
	switch val := v.Value.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func splitSQLColumns(s string) []string {
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
