package copy

import (
	"fmt"
	"strings"
	"unicode"
)

// Statement holds parsed COPY command options.
type Statement struct {
	TableName  string
	Columns    []string // nil means all columns
	Direction  string   // "from" or "to"
	Source     string   // "stdin" or a filepath
	Format     string   // "text" (default) or "csv"
	Delimiter  rune     // '\t' default for text, ',' default for csv
	NullString string   // `\N` default for text, "" default for csv
	Quote      rune     // '"' default for csv
	Escape     rune     // '"' default for csv
	Header     bool
}

// Parse parses a COPY SQL statement. Returns nil if sql is not a COPY statement.
func Parse(sql string) (*Statement, error) {
	sql = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))
	tokens := tokenize(sql)
	if len(tokens) == 0 || !strings.EqualFold(tokens[0], "COPY") {
		return nil, nil
	}

	s := &Statement{
		Direction: "from",
		Format:    "text",
		Delimiter: '\t',
		NullString: `\N`,
		Quote:     '"',
		Escape:    '"',
	}

	i := 1
	if i >= len(tokens) {
		return nil, fmt.Errorf("COPY: missing table name")
	}

	// table name (may be schema.table)
	s.TableName = stripSchema(tokens[i])
	i++

	// optional column list: (col1, col2, ...)
	if i < len(tokens) && tokens[i] == "(" {
		i++
		for i < len(tokens) && tokens[i] != ")" {
			col := strings.TrimSuffix(tokens[i], ",")
			if col != "" && col != "," {
				s.Columns = append(s.Columns, col)
			}
			i++
		}
		if i < len(tokens) {
			i++ // consume ")"
		}
	}

	if i >= len(tokens) {
		return nil, fmt.Errorf("COPY: missing FROM/TO")
	}

	dir := strings.ToLower(tokens[i])
	if dir != "from" && dir != "to" {
		return nil, fmt.Errorf("COPY: expected FROM or TO, got %q", tokens[i])
	}
	s.Direction = dir
	i++

	if i >= len(tokens) {
		return nil, fmt.Errorf("COPY: missing source")
	}

	src := strings.ToLower(tokens[i])
	if src == "stdin" {
		s.Source = "stdin"
	} else {
		s.Source = strings.Trim(tokens[i], "'\"")
	}
	i++

	// options: WITH (...) or bare keywords
	if i < len(tokens) && strings.EqualFold(tokens[i], "WITH") {
		i++
	}
	// strip surrounding parens if present
	if i < len(tokens) && tokens[i] == "(" {
		i++
		for i < len(tokens) && tokens[i] != ")" {
			i = parseOption(s, tokens, i)
		}
		if i < len(tokens) {
			i++ // consume ")"
		}
	} else {
		for i < len(tokens) {
			i = parseOption(s, tokens, i)
		}
	}

	// apply format defaults
	if s.Format == "csv" {
		if s.Delimiter == '\t' {
			s.Delimiter = ','
		}
		if s.NullString == `\N` {
			s.NullString = ""
		}
	}

	return s, nil
}

func parseOption(s *Statement, tokens []string, i int) int {
	kw := strings.ToUpper(tokens[i])
	i++
	switch kw {
	case "FORMAT":
		if i < len(tokens) {
			s.Format = strings.ToLower(tokens[i])
			i++
		}
	case "CSV":
		s.Format = "csv"
	case "BINARY":
		s.Format = "binary"
	case "TEXT":
		s.Format = "text"
	case "DELIMITER":
		if i < len(tokens) {
			d := strings.Trim(tokens[i], "'\"")
			if len(d) > 0 {
				s.Delimiter = rune(d[0])
			}
			i++
		}
	case "NULL":
		if i < len(tokens) {
			s.NullString = strings.Trim(tokens[i], "'\"")
			i++
		}
	case "QUOTE":
		if i < len(tokens) {
			q := strings.Trim(tokens[i], "'\"")
			if len(q) > 0 {
				s.Quote = rune(q[0])
			}
			i++
		}
	case "ESCAPE":
		if i < len(tokens) {
			e := strings.Trim(tokens[i], "'\"")
			if len(e) > 0 {
				s.Escape = rune(e[0])
			}
			i++
		}
	case "HEADER":
		s.Header = true
		// optional ON/OFF/TRUE/FALSE after HEADER
		if i < len(tokens) {
			v := strings.ToLower(tokens[i])
			if v == "on" || v == "true" || v == "1" {
				s.Header = true
				i++
			} else if v == "off" || v == "false" || v == "0" {
				s.Header = false
				i++
			}
		}
	case "FREEZE", "OIDS", "FORCE_QUOTE", "FORCE_NOT_NULL", "FORCE_NULL", "ENCODING":
		// skip option value if present
		if i < len(tokens) && tokens[i] != "," && !isKeyword(tokens[i]) {
			i++
		}
	}
	// skip trailing comma
	if i < len(tokens) && tokens[i] == "," {
		i++
	}
	return i
}

// ParseTextRow parses one line of COPY TEXT format.
// Returns (fields, isEOF, error).
// isEOF is true when the line is the end-of-data marker `\.`.
func ParseTextRow(line string, delimiter rune, nullStr string) ([]string, bool, error) {
	if line == `\.` {
		return nil, true, nil
	}
	parts := splitByDelimiter(line, delimiter)
	fields := make([]string, len(parts))
	for i, p := range parts {
		fields[i] = unescapeText(p, nullStr)
	}
	return fields, false, nil
}

// ParseCSVRow parses one line of COPY CSV format.
// Returns (fields, isEOF, error). isEOF is always false for CSV.
func ParseCSVRow(line string, delimiter rune, nullStr string, quote, escape rune) ([]string, bool, error) {
	var fields []string
	i := 0
	runes := []rune(line)
	n := len(runes)

	for {
		if i >= n {
			fields = append(fields, nullStr) // trailing delimiter → null-like empty
			break
		}
		if runes[i] == quote {
			// quoted field
			i++
			var sb strings.Builder
			for i < n {
				ch := runes[i]
				if ch == escape && i+1 < n && runes[i+1] == quote {
					sb.WriteRune(quote)
					i += 2
				} else if ch == quote {
					i++
					break
				} else {
					sb.WriteRune(ch)
					i++
				}
			}
			fields = append(fields, sb.String())
			// skip to delimiter
			if i < n && runes[i] == delimiter {
				i++
				if i == n {
					// trailing delimiter
					fields = append(fields, "")
					break
				}
			} else {
				break
			}
		} else {
			// unquoted field
			start := i
			for i < n && runes[i] != delimiter {
				i++
			}
			val := string(runes[start:i])
			if i < n && runes[i] == delimiter {
				i++
			}
			fields = append(fields, val)
			if i >= n && runes[i-1] == delimiter {
				fields = append(fields, "")
				break
			}
			if i >= n {
				break
			}
		}
	}

	// map null string to SQL NULL marker
	for i2, f := range fields {
		if f == nullStr {
			fields[i2] = "\x00NULL\x00" // internal NULL marker
		}
	}
	return fields, false, nil
}

// IsNullMarker returns true if the field value is the internal NULL marker.
func IsNullMarker(s string) bool {
	return s == "\x00NULL\x00"
}

// splitByDelimiter splits s by delimiter rune, not splitting inside nothing special—
// TEXT format has no quoting.
func splitByDelimiter(s string, delimiter rune) []string {
	if delimiter == '\t' {
		return strings.Split(s, "\t")
	}
	return strings.Split(s, string(delimiter))
}

// unescapeText handles PostgreSQL COPY TEXT escape sequences and NULL.
func unescapeText(s string, nullStr string) string {
	if s == nullStr {
		return "\x00NULL\x00"
	}
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) {
			i++
			switch runes[i] {
			case '\\':
				b.WriteRune('\\')
			case 'n':
				b.WriteRune('\n')
			case 't':
				b.WriteRune('\t')
			case 'r':
				b.WriteRune('\r')
			case 'b':
				b.WriteRune('\b')
			case 'f':
				b.WriteRune('\f')
			case 'v':
				b.WriteRune('\v')
			case 'N':
				// \N is NULL
				return "\x00NULL\x00"
			default:
				b.WriteRune('\\')
				b.WriteRune(runes[i])
			}
		} else {
			b.WriteRune(runes[i])
		}
	}
	return b.String()
}

func stripSchema(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

var keywords = map[string]bool{
	"FROM": true, "TO": true, "WITH": true, "FORMAT": true, "CSV": true,
	"TEXT": true, "BINARY": true, "DELIMITER": true, "NULL": true,
	"QUOTE": true, "ESCAPE": true, "HEADER": true, "FREEZE": true,
	"OIDS": true, "ENCODING": true, "FORCE_QUOTE": true, "FORCE_NOT_NULL": true,
	"FORCE_NULL": true, "STDIN": true, "STDOUT": true,
}

func isKeyword(s string) bool {
	return keywords[strings.ToUpper(s)]
}

// tokenize splits SQL into tokens, preserving quoted strings as single tokens.
func tokenize(sql string) []string {
	var tokens []string
	i := 0
	runes := []rune(sql)
	n := len(runes)

	for i < n {
		ch := runes[i]
		if unicode.IsSpace(ch) {
			i++
			continue
		}
		if ch == '\'' || ch == '"' {
			quote := ch
			j := i + 1
			for j < n && !(runes[j] == quote && (j+1 >= n || runes[j+1] != quote)) {
				if runes[j] == quote {
					j++ // skip doubled quote
				}
				j++
			}
			j++ // closing quote
			tokens = append(tokens, string(runes[i:j]))
			i = j
			continue
		}
		if ch == '(' || ch == ')' || ch == ',' {
			tokens = append(tokens, string(ch))
			i++
			continue
		}
		// word
		j := i
		for j < n && !unicode.IsSpace(runes[j]) && runes[j] != '(' && runes[j] != ')' && runes[j] != ',' && runes[j] != '\'' && runes[j] != '"' {
			j++
		}
		tokens = append(tokens, string(runes[i:j]))
		i = j
	}
	return tokens
}
