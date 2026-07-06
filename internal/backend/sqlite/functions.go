package sqlite

import (
	"crypto/md5"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"regexp"
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
	sqlite.MustRegisterDeterministicScalarFunction("left", 2, leftFunc)
	sqlite.MustRegisterDeterministicScalarFunction("lpad", 3, lpadFunc)
	sqlite.MustRegisterDeterministicScalarFunction("md5", 1, md5Func)
	sqlite.MustRegisterDeterministicScalarFunction("pg_numa_available", 0, pgNumaAvailableFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pg_input_is_valid", 2, pgInputIsValidFunc)
	sqlite.MustRegisterDeterministicScalarFunction("point", 2, pointFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_match", 3, regexpMatchFunc)
	sqlite.MustRegisterDeterministicScalarFunction("repeat", 2, repeatFunc)
	sqlite.MustRegisterDeterministicScalarFunction("reverse", 1, reverseFunc)
	sqlite.MustRegisterDeterministicScalarFunction("right", 2, rightFunc)
	sqlite.MustRegisterDeterministicScalarFunction("rpad", 3, rpadFunc)
	sqlite.MustRegisterDeterministicScalarFunction("lseg", 2, lsegFunc)
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
			return valueString(args[0]), nil
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
	if args[0] == nil {
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

func lpadFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	return padFunc("lpad", args, true)
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

func pointFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("point requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	return "(" + valueString(args[0]) + "," + valueString(args[1]) + ")", nil
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
	var b strings.Builder
	fillRunes := []rune(fill)
	for utf8.RuneCountInString(b.String()) < width {
		for _, r := range fillRunes {
			if utf8.RuneCountInString(b.String()) >= width {
				return b.String()
			}
			b.WriteRune(r)
		}
	}
	return string([]rune(b.String())[:width])
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
