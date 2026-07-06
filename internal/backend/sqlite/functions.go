package sqlite

import (
	"crypto/md5"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"modernc.org/sqlite"
)

var regexCache sync.Map

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("ascii", 1, asciiFunc)
	sqlite.MustRegisterDeterministicScalarFunction("btrim", -1, btrimFunc)
	sqlite.MustRegisterDeterministicScalarFunction("char_length", 1, charLengthFunc)
	sqlite.MustRegisterDeterministicScalarFunction("character_length", 1, charLengthFunc)
	sqlite.MustRegisterDeterministicScalarFunction("chr", 1, chrFunc)
	sqlite.MustRegisterDeterministicScalarFunction("convert_from", 2, convertFromFunc)
	sqlite.MustRegisterDeterministicScalarFunction("getdatabaseencoding", 0, getDatabaseEncodingFunc)
	sqlite.MustRegisterDeterministicScalarFunction("isclosed", 1, isClosedFunc)
	sqlite.MustRegisterDeterministicScalarFunction("isopen", 1, isOpenFunc)
	sqlite.MustRegisterDeterministicScalarFunction("left", 2, leftFunc)
	sqlite.MustRegisterDeterministicScalarFunction("line", 2, lineFunc)
	sqlite.MustRegisterDeterministicScalarFunction("lpad", 3, lpadFunc)
	sqlite.MustRegisterDeterministicScalarFunction("macaddr8_set7bit", 1, macaddr8Set7BitFunc)
	sqlite.MustRegisterDeterministicScalarFunction("md5", 1, md5Func)
	sqlite.MustRegisterDeterministicScalarFunction("pg_numa_available", 0, pgNumaAvailableFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pg_input_is_valid", 2, pgInputIsValidFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pg_relation_size", 1, pgRelationSizeFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pclose", 1, pcloseFunc)
	sqlite.MustRegisterDeterministicScalarFunction("point", 2, pointFunc)
	sqlite.MustRegisterDeterministicScalarFunction("popen", 1, popenFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_match", 3, regexpMatchFunc)
	sqlite.MustRegisterDeterministicScalarFunction("repeat", 2, repeatFunc)
	sqlite.MustRegisterDeterministicScalarFunction("reverse", 1, reverseFunc)
	sqlite.MustRegisterDeterministicScalarFunction("right", 2, rightFunc)
	sqlite.MustRegisterDeterministicScalarFunction("rpad", 3, rpadFunc)
	sqlite.MustRegisterDeterministicScalarFunction("test_translation", 0, testTranslationFunc)
	sqlite.MustRegisterDeterministicScalarFunction("lseg", 2, lsegFunc)
	sqlite.MustRegisterDeterministicScalarFunction("version", 0, versionFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_replace", -1, regexpReplaceFunc)
	sqlite.MustRegisterDeterministicScalarFunction("cashlarger", 2, cashlargerFunc)
	sqlite.MustRegisterDeterministicScalarFunction("cashsmaller", 2, cashsmallerFunc)
	sqlite.MustRegisterDeterministicScalarFunction("cash_words", 1, cashWordsFunc)
	sqlite.MustRegisterDeterministicScalarFunction("btoid8cmp", 2, btoid8cmpFunc)
	sqlite.MustRegisterDeterministicScalarFunction("center", 1, centerFunc)
	sqlite.MustRegisterDeterministicScalarFunction("radius", 1, radiusFunc)
	sqlite.MustRegisterDeterministicScalarFunction("diameter", 1, diameterFunc)
	sqlite.MustRegisterDeterministicScalarFunction("area", 1, areaFunc)
	sqlite.MustRegisterDeterministicScalarFunction("dist_op", 2, distOpFunc)
	sqlite.MustRegisterDeterministicScalarFunction("is_valid_circle", 1, isValidCircleFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pg_notify", 2, pgNotifyFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pg_notification_queue_usage", 0, pgNotificationQueueUsageFunc)
}

func asciiFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ascii requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	s := valueString(args[0])
	if s == "" {
		return int64(0), nil
	}
	r, _ := utf8.DecodeRuneInString(s)
	return int64(r), nil
}

func btrimFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return nil, fmt.Errorf("btrim requires 1 or 2 arguments")
	}
	if args[0] == nil {
		return nil, nil
	}
	cutset := " "
	if len(args) == 2 {
		if args[1] == nil {
			return nil, nil
		}
		cutset = valueString(args[1])
	}
	return strings.Trim(valueString(args[0]), cutset), nil
}

func charLengthFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("char_length requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	return int64(utf8.RuneCountInString(valueString(args[0]))), nil
}

func chrFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("chr requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	r := rune(valueInt(args[0]))
	if r == 0 || !utf8.ValidRune(r) {
		return nil, fmt.Errorf("requested character too large for encoding")
	}
	return string(r), nil
}

func convertFromFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("convert_from requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	return valueString(args[0]), nil
}

func leftFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("left requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	runes := []rune(valueString(args[0]))
	n := int(valueInt(args[1]))
	if n < 0 {
		n = len(runes) + n
	}
	if n < 0 {
		n = 0
	}
	if n > len(runes) {
		n = len(runes)
	}
	return string(runes[:n]), nil
}

func isClosedFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isclosed requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	s := strings.TrimSpace(valueString(args[0]))
	if strings.HasPrefix(s, "[") {
		return int64(1), nil
	}
	return int64(0), nil
}

func isOpenFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isopen requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	s := strings.TrimSpace(valueString(args[0]))
	if strings.HasPrefix(s, "(") {
		return int64(1), nil
	}
	return int64(0), nil
}

func lineFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("line requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	return valueString(args[0]) + "," + valueString(args[1]), nil
}

func lpadFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	return padFunc("lpad", args, true)
}

func macaddr8Set7BitFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("macaddr8_set7bit requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	return valueString(args[0]), nil
}

func md5Func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("md5 requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	sum := md5.Sum([]byte(valueString(args[0])))
	return hex.EncodeToString(sum[:]), nil
}

func lsegFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("lseg requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	return "[" + valueString(args[0]) + "," + valueString(args[1]) + "]", nil
}

func getDatabaseEncodingFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("getdatabaseencoding requires 0 arguments")
	}
	return "UTF8", nil
}

func pgNumaAvailableFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("pg_numa_available requires 0 arguments")
	}
	return int64(0), nil
}

func pgInputIsValidFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("pg_input_is_valid requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	return int64(1), nil
}

func pgRelationSizeFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("pg_relation_size requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	return int64(0), nil
}

func pcloseFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("pclose requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	s := strings.TrimSpace(valueString(args[0]))
	s = strings.TrimPrefix(strings.TrimSuffix(s, ")"), "(")
	s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
	return "[" + s + "]", nil
}

func pointFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("point requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	return "(" + valueString(args[0]) + "," + valueString(args[1]) + ")", nil
}

func popenFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("popen requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	s := strings.TrimSpace(valueString(args[0]))
	s = strings.TrimPrefix(strings.TrimSuffix(s, ")"), "(")
	s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
	return "(" + s + ")", nil
}

func repeatFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("repeat requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	count := int(valueInt(args[1]))
	if count <= 0 {
		return "", nil
	}
	return strings.Repeat(valueString(args[0]), count), nil
}

func reverseFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("reverse requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	runes := []rune(valueString(args[0]))
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes), nil
}

func rightFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("right requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	runes := []rune(valueString(args[0]))
	n := int(valueInt(args[1]))
	if n < 0 {
		n = len(runes) + n
	}
	if n < 0 {
		n = 0
	}
	if n > len(runes) {
		n = len(runes)
	}
	return string(runes[len(runes)-n:]), nil
}

func rpadFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	return padFunc("rpad", args, false)
}

func regexpMatchFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("regexp_match requires 3 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}

	pattern := valueString(args[0])
	text := valueString(args[1])
	flags := valueString(args[2])

	cacheKey := flags + "\x00" + pattern
	var rx *regexp.Regexp
	if val, ok := regexCache.Load(cacheKey); ok {
		rx = val.(*regexp.Regexp)
	} else {
		pat := pattern
		if flags == "i" {
			pat = "(?i)" + pat
		}
		var err error
		rx, err = regexp.Compile(pat)
		if err != nil {
			return nil, nil
		}
		regexCache.Store(cacheKey, rx)
	}

	if rx.MatchString(text) {
		return int64(1), nil
	}
	return int64(0), nil
}

func testTranslationFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("test_translation requires 0 arguments")
	}
	return nil, nil
}

func padFunc(name string, args []driver.Value, left bool) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("%s requires 3 arguments", name)
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	s := valueString(args[0])
	target := int(valueInt(args[1]))
	fill := valueString(args[2])
	if target <= 0 {
		return "", nil
	}
	runes := []rune(s)
	if len(runes) >= target {
		return string(runes[:target]), nil
	}
	if fill == "" {
		return s, nil
	}
	pad := buildPad(fill, target-len(runes))
	if left {
		return pad + s, nil
	}
	return s + pad, nil
}

func buildPad(fill string, width int) string {
	if width <= 0 || fill == "" {
		return ""
	}
	fillRunes := []rune(fill)
	if len(fillRunes) == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		r := fillRunes[i%len(fillRunes)]
		b.WriteRune(r)
	}
	return b.String()
}

func valueInt(v driver.Value) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case string:
		var n int64
		_, _ = fmt.Sscan(val, &n)
		return n
	default:
		var n int64
		_, _ = fmt.Sscan(fmt.Sprint(val), &n)
		return n
	}
}

func valueString(v driver.Value) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func versionFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	return "PostgreSQL 14.0 compatible minipsql", nil
}

func regexpReplaceFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 && len(args) != 4 {
		return nil, fmt.Errorf("regexp_replace requires 3 or 4 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	source := valueString(args[0])
	pattern := valueString(args[1])
	replacement := valueString(args[2])
	flags := ""
	if len(args) == 4 {
		if args[3] == nil {
			return nil, nil
		}
		flags = valueString(args[3])
	}

	cacheKey := flags + "\x00" + pattern
	var rx *regexp.Regexp
	if val, ok := regexCache.Load(cacheKey); ok {
		rx = val.(*regexp.Regexp)
	} else {
		pat := pattern
		actualFlags := ""
		if strings.Contains(flags, "i") {
			actualFlags += "i"
		}
		if actualFlags != "" {
			pat = "(?" + actualFlags + ")" + pat
		}
		var err error
		rx, err = regexp.Compile(pat)
		if err != nil {
			return nil, err
		}
		regexCache.Store(cacheKey, rx)
	}

	if strings.Contains(flags, "g") {
		return rx.ReplaceAllString(source, replacement), nil
	} else {
		loc := rx.FindStringIndex(source)
		if loc == nil {
			return source, nil
		}
		match := source[loc[0]:loc[1]]
		replaced := rx.ReplaceAllString(match, replacement)
		return source[:loc[0]] + replaced + source[loc[1]:], nil
	}
}

func parseMoney(v driver.Value) (float64, string, bool) {
	if v == nil {
		return 0, "", false
	}
	s := valueString(v)
	clean := strings.ReplaceAll(s, "$", "")
	clean = strings.ReplaceAll(clean, ",", "")
	clean = strings.TrimSpace(clean)
	f, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, s, false
	}
	return f, s, true
}

func cashlargerFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("cashlarger requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	f1, s1, ok1 := parseMoney(args[0])
	f2, s2, ok2 := parseMoney(args[1])
	if !ok1 || !ok2 {
		if s1 > s2 {
			return s1, nil
		}
		return s2, nil
	}
	if f1 > f2 {
		return args[0], nil
	}
	return args[1], nil
}

func cashsmallerFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("cashsmaller requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	f1, s1, ok1 := parseMoney(args[0])
	f2, s2, ok2 := parseMoney(args[1])
	if !ok1 || !ok2 {
		if s1 < s2 {
			return s1, nil
		}
		return s2, nil
	}
	if f1 < f2 {
		return args[0], nil
	}
	return args[1], nil
}

func intToWords(n int64) string {
	if n == 0 {
		return ""
	}

	units := []string{"", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
		"eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	tens := []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}

	var parts []string

	if n >= 1000000000 {
		parts = append(parts, intToWords(n/1000000000)+" billion")
		n %= 1000000000
	}
	if n >= 1000000 {
		parts = append(parts, intToWords(n/1000000)+" million")
		n %= 1000000
	}
	if n >= 1000 {
		parts = append(parts, intToWords(n/1000)+" thousand")
		n %= 1000
	}
	if n >= 100 {
		parts = append(parts, units[n/100]+" hundred")
		n %= 100
	}
	if n >= 20 {
		part := tens[n/10]
		if n%10 > 0 {
			part += " " + units[n%10]
		}
		parts = append(parts, part)
	} else if n > 0 {
		parts = append(parts, units[n])
	}

	return strings.Join(parts, " ")
}

func cashWordsFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("cash_words requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	f, _, ok := parseMoney(args[0])
	if !ok {
		return nil, fmt.Errorf("invalid money value: %v", args[0])
	}

	negative := f < 0
	if negative {
		f = -f
	}

	dollars := int64(f)
	cents := int64(math.Round((f - float64(dollars)) * 100))

	var dollarsStr string
	if dollars == 0 {
		dollarsStr = "Zero"
	} else {
		dollarsStr = intToWords(dollars)
		if len(dollarsStr) > 0 {
			dollarsStr = strings.ToUpper(dollarsStr[:1]) + dollarsStr[1:]
		}
	}

	var centsStr string
	if cents == 0 {
		centsStr = "zero"
	} else {
		centsStr = intToWords(cents)
	}

	res := fmt.Sprintf("%s dollars and %s cents", dollarsStr, centsStr)
	if negative {
		res = "Minus " + res
	}
	return res, nil
}

func btoid8cmpFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("btoid8cmp requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	v1 := valueInt(args[0])
	v2 := valueInt(args[1])
	if v1 < v2 {
		return int64(-1), nil
	} else if v1 > v2 {
		return int64(1), nil
	}
	return int64(0), nil
}

func parseCircle(v driver.Value) (x, y, r float64, ok bool) {
	if v == nil {
		return 0, 0, 0, false
	}
	s := valueString(v)
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		s = s[1 : len(s)-1]
		s = strings.TrimSpace(s)
	}

	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
		s = strings.TrimSpace(s)
	}

	idx := strings.LastIndex(s, ",")
	if idx == -1 {
		return 0, 0, 0, false
	}
	rStr := strings.TrimSpace(s[idx+1:])
	pointStr := strings.TrimSpace(s[:idx])

	if strings.HasPrefix(pointStr, "(") && strings.HasSuffix(pointStr, ")") {
		pointStr = pointStr[1 : len(pointStr)-1]
		pointStr = strings.TrimSpace(pointStr)
	}

	parts := strings.Split(pointStr, ",")
	if len(parts) != 2 {
		return 0, 0, 0, false
	}
	xVal, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, 0, false
	}
	yVal, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, 0, false
	}
	rVal, err := strconv.ParseFloat(rStr, 64)
	if err != nil {
		if strings.ToLower(rStr) == "nan" {
			return xVal, yVal, math.NaN(), true
		}
		return 0, 0, 0, false
	}
	if !math.IsNaN(rVal) && rVal < 0 {
		return 0, 0, 0, false
	}
	return xVal, yVal, rVal, true
}

func parsePoint(s string) (x, y float64, ok bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
	}
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	xVal, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, false
	}
	yVal, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, false
	}
	return xVal, yVal, true
}

func centerFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("center requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	x, y, _, ok := parseCircle(args[0])
	if !ok {
		return nil, fmt.Errorf("invalid circle: %v", args[0])
	}
	return fmt.Sprintf("(%g,%g)", x, y), nil
}

func radiusFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("radius requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	_, _, r, ok := parseCircle(args[0])
	if !ok {
		return nil, fmt.Errorf("invalid circle: %v", args[0])
	}
	if math.IsNaN(r) {
		return "NaN", nil
	}
	return r, nil
}

func diameterFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("diameter requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	_, _, r, ok := parseCircle(args[0])
	if !ok {
		return nil, fmt.Errorf("invalid circle: %v", args[0])
	}
	if math.IsNaN(r) {
		return "NaN", nil
	}
	return 2 * r, nil
}

func areaFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("area requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	_, _, r, ok := parseCircle(args[0])
	if !ok {
		return nil, fmt.Errorf("invalid circle: %v", args[0])
	}
	if math.IsNaN(r) {
		return "NaN", nil
	}
	return math.Pi * r * r, nil
}

func distOpFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("dist_op requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	s1 := valueString(args[0])
	s2 := valueString(args[1])

	x1, y1, r1, ok1 := parseCircle(args[0])
	x2, y2, r2, ok2 := parseCircle(args[1])
	if ok1 && ok2 {
		dx := x1 - x2
		dy := y1 - y2
		dist := math.Sqrt(dx*dx + dy*dy)
		return dist - (r1 + r2), nil
	}

	px1, py1, pok1 := parsePoint(s1)
	px2, py2, pok2 := parsePoint(s2)
	if pok1 && pok2 {
		dx := px1 - px2
		dy := py1 - py2
		return math.Sqrt(dx*dx + dy*dy), nil
	}

	if ok1 && pok2 {
		dx := x1 - px2
		dy := y1 - py2
		return math.Sqrt(dx*dx+dy*dy) - r1, nil
	}
	if pok1 && ok2 {
		dx := px1 - x2
		dy := py1 - y2
		return math.Sqrt(dx*dx+dy*dy) - r2, nil
	}

	return 0.0, nil
}

func isValidCircleFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("is_valid_circle requires 1 argument")
	}
	if args[0] == nil {
		return int64(1), nil
	}
	val := valueString(args[0])
	_, _, _, ok := parseCircle(args[0])
	if !ok {
		return nil, fmt.Errorf("invalid input syntax for type circle: %q", val)
	}
	return int64(1), nil
}

func pgNotifyFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("pg_notify requires 2 arguments")
	}
	if args[0] == nil {
		return nil, fmt.Errorf("channel name cannot be empty")
	}
	channel := valueString(args[0])
	if channel == "" {
		return nil, fmt.Errorf("channel name cannot be empty")
	}
	if len(channel) > 63 {
		return nil, fmt.Errorf("channel name too long")
	}
	return nil, nil
}

func pgNotificationQueueUsageFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	return int64(0), nil
}
