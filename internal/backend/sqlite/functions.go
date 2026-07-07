package sqlite

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql/driver"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"math"
	"math/big"
	"math/bits"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"modernc.org/sqlite"

	"github.com/yashikota/minipsql/internal/compat"
)

var regexCache sync.Map

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("ascii", 1, asciiFunc)
	sqlite.MustRegisterDeterministicScalarFunction("btrim", -1, btrimFunc)
	sqlite.MustRegisterDeterministicScalarFunction("char_length", 1, charLengthFunc)
	sqlite.MustRegisterDeterministicScalarFunction("character_length", 1, charLengthFunc)
	sqlite.MustRegisterDeterministicScalarFunction("encode", 2, encodeFunc)
	sqlite.MustRegisterDeterministicScalarFunction("decode", 2, decodeFunc)
	sqlite.MustRegisterDeterministicScalarFunction("chr", 1, chrFunc)
	sqlite.MustRegisterDeterministicScalarFunction("convert_from", 2, convertFromFunc)
	sqlite.MustRegisterDeterministicScalarFunction("getdatabaseencoding", 0, getDatabaseEncodingFunc)
	sqlite.MustRegisterDeterministicScalarFunction("isclosed", 1, isClosedFunc)
	sqlite.MustRegisterDeterministicScalarFunction("isopen", 1, isOpenFunc)
	sqlite.MustRegisterDeterministicScalarFunction("left", 2, leftFunc)
	sqlite.MustRegisterDeterministicScalarFunction("line", 2, lineFunc)
	sqlite.MustRegisterDeterministicScalarFunction("initcap", 1, initcapFunc)
	sqlite.MustRegisterDeterministicScalarFunction("lpad", -1, lpadFunc)
	sqlite.MustRegisterDeterministicScalarFunction("ltrim", -1, ltrimFunc)
	sqlite.MustRegisterDeterministicScalarFunction("rtrim", -1, rtrimFunc)
	sqlite.MustRegisterDeterministicScalarFunction("macaddr8_set7bit", 1, macaddr8Set7BitFunc)
	sqlite.MustRegisterDeterministicScalarFunction("md5", 1, md5Func)
	sqlite.MustRegisterDeterministicScalarFunction("pg_numa_available", 0, pgNumaAvailableFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pg_input_is_valid", 2, pgInputIsValidFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pg_relation_size", 1, pgRelationSizeFunc)
	sqlite.MustRegisterDeterministicScalarFunction("pclose", 1, pcloseFunc)
	sqlite.MustRegisterDeterministicScalarFunction("point", 2, pointFunc)
	sqlite.MustRegisterDeterministicScalarFunction("popen", 1, popenFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_regexp_match", 3, regexpMatchFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_match", -1, pgRegexpMatchFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_like", -1, regexpLikeFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_count", -1, regexpCountFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_instr", -1, regexpInstrFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_substr", -1, regexpSubstrFunc)
	sqlite.MustRegisterDeterministicScalarFunction("regexp_split_to_array", -1, regexpSplitToArrayFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_regexp_split_to_json", -1, minipsqlRegexpSplitToJSONFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_regexp_matches_to_json", -1, minipsqlRegexpMatchesToJSONFunc)
	sqlite.MustRegisterDeterministicScalarFunction("repeat", 2, repeatFunc)
	sqlite.MustRegisterDeterministicScalarFunction("reverse", 1, reverseFunc)
	sqlite.MustRegisterDeterministicScalarFunction("strpos", 2, strposFunc)
	sqlite.MustRegisterDeterministicScalarFunction("split_part", 3, splitPartFunc)
	sqlite.MustRegisterDeterministicScalarFunction("get_bit", 2, getBitFunc)
	sqlite.MustRegisterDeterministicScalarFunction("set_bit", 3, setBitFunc)
	sqlite.MustRegisterDeterministicScalarFunction("get_byte", 2, getByteFunc)
	sqlite.MustRegisterDeterministicScalarFunction("set_byte", 3, setByteFunc)
	sqlite.MustRegisterDeterministicScalarFunction("crc32", 1, crc32Func)
	sqlite.MustRegisterDeterministicScalarFunction("crc32c", 1, crc32cFunc)
	sqlite.MustRegisterDeterministicScalarFunction("sha224", 1, sha224Func)
	sqlite.MustRegisterDeterministicScalarFunction("sha256", 1, sha256Func)
	sqlite.MustRegisterDeterministicScalarFunction("sha384", 1, sha384Func)
	sqlite.MustRegisterDeterministicScalarFunction("sha512", 1, sha512Func)
	sqlite.MustRegisterDeterministicScalarFunction("to_bin", 1, toBinFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_to_bin_int", 1, toBinFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_to_bin_bigint", 1, toBinBigintFunc)
	sqlite.MustRegisterDeterministicScalarFunction("to_oct", 1, toOctFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_to_oct_int", 1, toOctFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_to_oct_bigint", 1, toOctBigintFunc)
	sqlite.MustRegisterDeterministicScalarFunction("to_hex", 1, toHexFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_to_hex_int", 1, toHexFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_to_hex_bigint", 1, toHexBigintFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_overlay", -1, minipsqlOverlayFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_error", 3, minipsqlErrorFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_similar_substring", 3, minipsqlSimilarSubstringFunc)
	sqlite.MustRegisterDeterministicScalarFunction("minipsql_similar_to", 3, minipsqlSimilarToFunc)
	sqlite.MustRegisterDeterministicScalarFunction("booleq", 2, booleqFunc)
	sqlite.MustRegisterDeterministicScalarFunction("boolne", 2, boolneFunc)
	sqlite.MustRegisterDeterministicScalarFunction("gcd", 2, gcdFunc)
	sqlite.MustRegisterDeterministicScalarFunction("lcm", 2, lcmFunc)
	sqlite.MustRegisterDeterministicScalarFunction("right", 2, rightFunc)
	sqlite.MustRegisterDeterministicScalarFunction("rpad", -1, rpadFunc)
	sqlite.MustRegisterDeterministicScalarFunction("translate", 3, translateFunc)
	sqlite.MustRegisterDeterministicScalarFunction("bit_count", 1, bitCountFunc)
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
	if r == 0 {
		return nil, fmt.Errorf("null character not permitted")
	}
	if !utf8.ValidRune(r) || r < 0 || r > 0x10ffff {
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
	if len(args) != 2 && len(args) != 3 {
		return nil, fmt.Errorf("%s requires 2 or 3 arguments", name)
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	s := valueString(args[0])
	target := int(valueInt(args[1]))
	fill := " "
	if len(args) == 3 {
		if args[2] == nil {
			return nil, nil
		}
		fill = valueString(args[2])
	}
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
	if len(args) < 3 || len(args) > 6 {
		return nil, fmt.Errorf("regexp_replace requires between 3 and 6 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	replacement := valueString(args[2])

	position := int64(1)
	occurrence := int64(1)
	flags := ""

	switch len(args) {
	case 4:
		if isString(args[3]) {
			flags = valueString(args[3])
		} else {
			position = valueInt(args[3])
		}
	case 5:
		position = valueInt(args[3])
		if isString(args[4]) {
			flags = valueString(args[4])
		} else {
			occurrence = valueInt(args[4])
		}
	case 6:
		position = valueInt(args[3])
		occurrence = valueInt(args[4])
		flags = valueString(args[5])
	}

	pos := int(position) - 1
	if pos < 0 {
		pos = 0
	}
	runeText := []rune(text)
	if pos > len(runeText) {
		return text, nil
	}
	subText := string(runeText[pos:])

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}

	replaceAll := strings.Contains(flags, "g") || occurrence == 0

	var result string
	if replaceAll {
		replacedSub := rx.ReplaceAllString(subText, replacement)
		result = string(runeText[:pos]) + replacedSub
	} else {
		occ := int(occurrence)
		if occ <= 0 {
			occ = 1
		}
		matches := rx.FindAllStringSubmatchIndex(subText, -1)
		idx := occ - 1
		if idx < 0 || idx >= len(matches) {
			result = text
		} else {
			matchIdxs := matches[idx]
			startByte := matchIdxs[0]
			endByte := matchIdxs[1]
			matchStr := subText[startByte:endByte]
			replacedMatch := rx.ReplaceAllString(matchStr, replacement)
			result = string(runeText[:pos]) + subText[:startByte] + replacedMatch + subText[endByte:]
		}
	}

	return result, nil
}

func compileRegexp(pattern, flags string) (*regexp.Regexp, error) {
	cacheKey := flags + "\x00" + pattern
	if val, ok := regexCache.Load(cacheKey); ok {
		return val.(*regexp.Regexp), nil
	}

	pat := pattern
	actualFlags := ""
	if strings.Contains(flags, "i") {
		actualFlags += "i"
	}
	if strings.Contains(flags, "m") || strings.Contains(flags, "n") {
		actualFlags += "m"
	}
	if strings.Contains(flags, "s") {
		actualFlags += "s"
	}
	
	if actualFlags != "" {
		pat = "(?" + actualFlags + ")" + pat
	}

	rx, err := regexp.Compile(pat)
	if err != nil {
		return nil, err
	}
	regexCache.Store(cacheKey, rx)
	return rx, nil
}

func isString(v driver.Value) bool {
	switch v.(type) {
	case string, []byte:
		return true
	default:
		return false
	}
}

func formatPGTextArray(elements []string) string {
	var quoted []string
	for _, el := range elements {
		escaped := strings.ReplaceAll(el, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		quoted = append(quoted, "\""+escaped+"\"")
	}
	return "{" + strings.Join(quoted, ",") + "}"
}

func pgRegexpMatchFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nil, fmt.Errorf("regexp_match requires 2 or 3 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	flags := ""
	if len(args) == 3 {
		if args[2] == nil {
			return nil, nil
		}
		flags = valueString(args[2])
	}

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}

	match := rx.FindStringSubmatch(text)
	if match == nil {
		return nil, nil
	}

	var elements []string
	if len(match) > 1 {
		elements = match[1:]
	} else {
		elements = []string{match[0]}
	}

	return formatPGTextArray(elements), nil
}

func regexpLikeFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nil, fmt.Errorf("regexp_like requires 2 or 3 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	flags := ""
	if len(args) == 3 {
		if args[2] == nil {
			return nil, nil
		}
		flags = valueString(args[2])
	}

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}
	if rx.MatchString(text) {
		return int64(1), nil
	}
	return int64(0), nil
}

func regexpCountFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) < 2 || len(args) > 4 {
		return nil, fmt.Errorf("regexp_count requires between 2 and 4 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	position := int64(1)
	flags := ""

	if len(args) >= 3 && args[2] != nil {
		position = valueInt(args[2])
	}
	if len(args) == 4 && args[3] != nil {
		flags = valueString(args[3])
	}

	pos := int(position) - 1
	if pos < 0 {
		pos = 0
	}
	runeText := []rune(text)
	if pos > len(runeText) {
		return int64(0), nil
	}
	subText := string(runeText[pos:])

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}

	matches := rx.FindAllStringIndex(subText, -1)
	return int64(len(matches)), nil
}

func regexpInstrFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) < 2 || len(args) > 7 {
		return nil, fmt.Errorf("regexp_instr requires between 2 and 7 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	position := int64(1)
	occurrence := int64(1)
	returnOption := int64(0)
	flags := ""
	group := int64(0)

	if len(args) >= 3 && args[2] != nil {
		position = valueInt(args[2])
	}
	if len(args) >= 4 && args[3] != nil {
		occurrence = valueInt(args[3])
	}
	if len(args) >= 5 && args[4] != nil {
		returnOption = valueInt(args[4])
	}
	if len(args) >= 6 && args[5] != nil {
		flags = valueString(args[5])
	}
	if len(args) == 7 && args[6] != nil {
		group = valueInt(args[6])
	}

	if position <= 0 {
		return nil, fmt.Errorf("position must be greater than zero")
	}
	if occurrence <= 0 {
		return nil, fmt.Errorf("occurrence must be greater than zero")
	}
	if returnOption != 0 && returnOption != 1 {
		return nil, fmt.Errorf("return_option must be 0 or 1")
	}
	if group < 0 {
		return nil, fmt.Errorf("group must be greater than or equal to zero")
	}

	pos := int(position) - 1
	runeText := []rune(text)
	if pos > len(runeText) {
		return int64(0), nil
	}
	subText := string(runeText[pos:])

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}

	matches := rx.FindAllStringSubmatchIndex(subText, -1)
	occ := int(occurrence) - 1
	if occ < 0 || occ >= len(matches) {
		return int64(0), nil
	}

	matchIdxs := matches[occ]
	g := int(group)
	if 2*g >= len(matchIdxs) || matchIdxs[2*g] == -1 {
		return int64(0), nil
	}

	var targetByteOffset int
	if returnOption == 0 {
		targetByteOffset = matchIdxs[2*g]
	} else {
		targetByteOffset = matchIdxs[2*g+1]
	}

	runeOffsetInSubText := len([]rune(subText[:targetByteOffset]))
	resultRuneIndex := pos + runeOffsetInSubText + 1

	return int64(resultRuneIndex), nil
}

func regexpSubstrFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) < 2 || len(args) > 6 {
		return nil, fmt.Errorf("regexp_substr requires between 2 and 6 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	position := int64(1)
	occurrence := int64(1)
	flags := ""
	group := int64(0)

	if len(args) >= 3 && args[2] != nil {
		position = valueInt(args[2])
	}
	if len(args) >= 4 && args[3] != nil {
		occurrence = valueInt(args[3])
	}
	if len(args) >= 5 && args[4] != nil {
		flags = valueString(args[4])
	}
	if len(args) == 6 && args[5] != nil {
		group = valueInt(args[5])
	}

	if position <= 0 {
		return nil, fmt.Errorf("position must be greater than zero")
	}
	if occurrence <= 0 {
		return nil, fmt.Errorf("occurrence must be greater than zero")
	}
	if group < 0 {
		return nil, fmt.Errorf("group must be greater than or equal to zero")
	}

	pos := int(position) - 1
	runeText := []rune(text)
	if pos > len(runeText) {
		return nil, nil
	}
	subText := string(runeText[pos:])

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}

	matches := rx.FindAllStringSubmatchIndex(subText, -1)
	occ := int(occurrence) - 1
	if occ < 0 || occ >= len(matches) {
		return nil, nil
	}

	matchIdxs := matches[occ]
	g := int(group)
	if 2*g >= len(matchIdxs) || matchIdxs[2*g] == -1 {
		return nil, nil
	}

	startByte := matchIdxs[2*g]
	endByte := matchIdxs[2*g+1]
	return subText[startByte:endByte], nil
}

func regexpSplitToArrayFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nil, fmt.Errorf("regexp_split_to_array requires 2 or 3 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	flags := ""
	if len(args) == 3 {
		if args[2] == nil {
			return nil, nil
		}
		flags = valueString(args[2])
	}

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}

	parts := rx.Split(text, -1)
	return formatPGTextArray(parts), nil
}

func minipsqlRegexpSplitToJSONFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nil, fmt.Errorf("regexp_split_to_json requires 2 or 3 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return "[]", nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	flags := ""
	if len(args) == 3 {
		if args[2] == nil {
			return "[]", nil
		}
		flags = valueString(args[2])
	}

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}

	parts := rx.Split(text, -1)
	
	var quoted []string
	for _, p := range parts {
		escaped := strings.ReplaceAll(p, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		quoted = append(quoted, "\""+escaped+"\"")
	}
	return "[" + strings.Join(quoted, ",") + "]", nil
}

func minipsqlRegexpMatchesToJSONFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nil, fmt.Errorf("regexp_matches requires 2 or 3 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return "[]", nil
	}
	text := valueString(args[0])
	pattern := valueString(args[1])
	flags := ""
	if len(args) == 3 {
		if args[2] == nil {
			return "[]", nil
		}
		flags = valueString(args[2])
	}

	rx, err := compileRegexp(pattern, flags)
	if err != nil {
		return nil, err
	}

	global := strings.Contains(flags, "g")

	var matches [][]int
	if global {
		matches = rx.FindAllStringSubmatchIndex(text, -1)
	} else {
		submatch := rx.FindStringSubmatchIndex(text)
		if submatch != nil {
			matches = [][]int{submatch}
		}
	}

	var jsonParts []string
	for _, matchIdxs := range matches {
		var elements []string
		if len(matchIdxs) > 2 {
			for i := 1; i < len(matchIdxs)/2; i++ {
				startByte := matchIdxs[2*i]
				endByte := matchIdxs[2*i+1]
				if startByte == -1 {
					elements = append(elements, "NULL")
				} else {
					elements = append(elements, "\"" + escapePGArrayString(text[startByte:endByte]) + "\"")
				}
			}
		} else {
			startByte := matchIdxs[0]
			endByte := matchIdxs[1]
			elements = append(elements, "\"" + escapePGArrayString(text[startByte:endByte]) + "\"")
		}

		pgArrayStr := "{" + strings.Join(elements, ",") + "}"
		escapedJSON := strings.ReplaceAll(pgArrayStr, "\\", "\\\\")
		escapedJSON = strings.ReplaceAll(escapedJSON, "\"", "\\\"")
		jsonParts = append(jsonParts, "\""+escapedJSON+"\"")
	}

	return "[" + strings.Join(jsonParts, ",") + "]", nil
}

func escapePGArrayString(el string) string {
	escaped := strings.ReplaceAll(el, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return escaped
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

func encodeFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("encode requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	
	format := strings.ToLower(valueString(args[1]))
	switch format {
	case "hex":
		return hex.EncodeToString(data), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(data), nil
	case "base64url":
		return base64.URLEncoding.EncodeToString(data), nil
	case "base32hex":
		return base32.HexEncoding.EncodeToString(data), nil
	case "escape":
		var b strings.Builder
		for _, c := range data {
			if c == '\\' {
				b.WriteString("\\\\")
			} else if c < 32 || c > 126 {
				b.WriteString(fmt.Sprintf("\\%03o", c))
			} else {
				b.WriteByte(c)
			}
		}
		return b.String(), nil
	default:
		return nil, fmt.Errorf("unrecognized encoding: %q\nHINT: Valid encodings are \"base32hex\", \"base64\", \"base64url\", \"escape\", and \"hex\".", format)
	}
}

func decodeFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("decode requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	
	str := valueString(args[0])
	format := strings.ToLower(valueString(args[1]))
	
	switch format {
	case "hex":
		str = strings.ReplaceAll(str, " ", "")
		str = strings.ReplaceAll(str, "\n", "")
		str = strings.ReplaceAll(str, "\r", "")
		str = strings.ReplaceAll(str, "\t", "")
		data, err := hex.DecodeString(str)
		if err != nil {
			return nil, err
		}
		return data, nil
	case "base64":
		str = strings.ReplaceAll(str, " ", "")
		str = strings.ReplaceAll(str, "\n", "")
		str = strings.ReplaceAll(str, "\r", "")
		str = strings.ReplaceAll(str, "\t", "")
		// Validation
		{
			hasPadding := false
			numChars := 0
			for i := 0; i < len(str); i++ {
				c := str[i]
				if c == '=' {
					if numChars == 0 {
						return nil, fmt.Errorf("unexpected %q while decoding base64 sequence", "=")
					}
					rem := numChars % 4
					if rem != 0 && rem != 2 && rem != 3 {
						return nil, fmt.Errorf("unexpected %q while decoding base64 sequence", "=")
					}
					hasPadding = true
					continue
				}
				if hasPadding {
					return nil, fmt.Errorf("invalid symbol %q found while decoding base64 sequence", string(c))
				}
				isValid := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/'
				if !isValid {
					return nil, fmt.Errorf("invalid symbol %q found while decoding base64 sequence", string(c))
				}
				numChars++
			}
			if numChars % 4 == 1 {
				return nil, fmt.Errorf("invalid base64 end sequence\nHINT: Input data is missing padding, is truncated, or is otherwise corrupted.")
			}
		}
		data, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			return nil, err
		}
		return data, nil
	case "base64url":
		str = strings.ReplaceAll(str, " ", "")
		str = strings.ReplaceAll(str, "\n", "")
		str = strings.ReplaceAll(str, "\r", "")
		str = strings.ReplaceAll(str, "\t", "")
		// Validation
		{
			hasPadding := false
			numChars := 0
			for i := 0; i < len(str); i++ {
				c := str[i]
				if c == '=' {
					if numChars == 0 {
						return nil, fmt.Errorf("unexpected %q while decoding base64url sequence", "=")
					}
					rem := numChars % 4
					if rem != 0 && rem != 2 && rem != 3 {
						return nil, fmt.Errorf("unexpected %q while decoding base64url sequence", "=")
					}
					hasPadding = true
					continue
				}
				if hasPadding {
					return nil, fmt.Errorf("invalid symbol %q found while decoding base64url sequence", string(c))
				}
				isValid := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
				if !isValid {
					return nil, fmt.Errorf("invalid symbol %q found while decoding base64url sequence", string(c))
				}
				numChars++
			}
			if numChars % 4 == 1 {
				return nil, fmt.Errorf("invalid base64url end sequence\nHINT: Input data is missing padding, is truncated, or is otherwise corrupted.")
			}
		}
		stripped := strings.TrimRight(str, "=")
		data, err := base64.RawURLEncoding.DecodeString(stripped)
		if err != nil {
			data, err = base64.URLEncoding.DecodeString(str)
			if err != nil {
				return nil, err
			}
		}
		return data, nil
	case "base32hex":
		str = strings.ReplaceAll(str, " ", "")
		str = strings.ReplaceAll(str, "\n", "")
		str = strings.ReplaceAll(str, "\r", "")
		str = strings.ReplaceAll(str, "\t", "")
		str = strings.ToUpper(str)
		// Validation
		{
			hasPadding := false
			numChars := 0
			for i := 0; i < len(str); i++ {
				c := str[i]
				if c == '=' {
					if numChars == 0 {
						return nil, fmt.Errorf("unexpected %q while decoding base32hex sequence", "=")
					}
					rem := numChars % 8
					if rem != 0 && rem != 2 && rem != 4 && rem != 5 && rem != 7 {
						return nil, fmt.Errorf("unexpected %q while decoding base32hex sequence", "=")
					}
					hasPadding = true
					continue
				}
				if hasPadding {
					return nil, fmt.Errorf("invalid symbol %q found while decoding base32hex sequence", string(c))
				}
				isValid := (c >= '0' && c <= '9') || (c >= 'A' && c <= 'V')
				if !isValid {
					return nil, fmt.Errorf("invalid symbol %q found while decoding base32hex sequence", string(c))
				}
				numChars++
			}
		}
		str = strings.TrimRight(str, "=")
		enc := base32.HexEncoding.WithPadding(base32.NoPadding)
		data, err := enc.DecodeString(str)
		if err != nil {
			return nil, err
		}
		return data, nil
	case "escape":
		var res []byte
		i := 0
		for i < len(str) {
			if str[i] == '\\' && i+1 < len(str) {
				if str[i+1] == '\\' {
					res = append(res, '\\')
					i += 2
				} else if i+3 < len(str) && isOctalDigit(str[i+1]) && isOctalDigit(str[i+2]) && isOctalDigit(str[i+3]) {
					val, _ := strconv.ParseInt(str[i+1:i+4], 8, 32)
					res = append(res, byte(val))
					i += 4
				} else {
					res = append(res, str[i])
					i++
				}
			} else {
				res = append(res, str[i])
				i++
			}
		}
		return res, nil
	default:
		return nil, fmt.Errorf("unrecognized encoding: %q\nHINT: Valid encodings are \"base32hex\", \"base64\", \"base64url\", \"escape\", and \"hex\".", format)
	}
}

func isOctalDigit(c byte) bool {
	return c >= '0' && c <= '7'
}

func strposFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("strpos requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	s := valueString(args[0])
	sub := valueString(args[1])
	
	subRunes := []rune(sub)
	if len(subRunes) == 0 {
		return int64(1), nil
	}
	
	idx := strings.Index(s, sub)
	if idx == -1 {
		return int64(0), nil
	}
	
	runeIdx := len([]rune(s[:idx]))
	return int64(runeIdx + 1), nil
}

func minipsqlOverlayFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 && len(args) != 4 {
		return nil, fmt.Errorf("overlay requires 3 or 4 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	
	str := valueString(args[0])
	newstr := valueString(args[1])
	start := valueInt(args[2])
	
	strRunes := []rune(str)
	newstrRunes := []rune(newstr)
	
	startIdx := int(start) - 1
	if startIdx < 0 {
		startIdx = 0
	}
	
	var count int
	if len(args) == 4 {
		if args[3] == nil {
			return nil, nil
		}
		count = int(valueInt(args[3]))
		if count < 0 {
			return nil, fmt.Errorf("negative substring length not allowed")
		}
	} else {
		count = len(newstrRunes)
	}
	
	if startIdx > len(strRunes) {
		startIdx = len(strRunes)
	}
	
	endIdx := startIdx + count
	if endIdx > len(strRunes) {
		endIdx = len(strRunes)
	}
	
	result := append([]rune{}, strRunes[:startIdx]...)
	result = append(result, newstrRunes...)
	result = append(result, strRunes[endIdx:]...)
	return string(result), nil
}

func splitPartFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("split_part requires 3 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	s := valueString(args[0])
	delim := valueString(args[1])
	field := valueInt(args[2])
	
	if field <= 0 {
		return nil, fmt.Errorf("field position must be greater than zero")
	}
	
	if delim == "" {
		if field == 1 {
			return s, nil
		}
		return "", nil
	}
	
	parts := strings.Split(s, delim)
	if int(field) > len(parts) {
		return "", nil
	}
	return parts[field-1], nil
}

func toBinFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("to_bin requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	val := valueInt(args[0])
	if val < 0 {
		return strconv.FormatUint(uint64(uint32(val)), 2), nil
	}
	return strconv.FormatInt(val, 2), nil
}

func toBinBigintFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("to_bin requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	val := valueInt(args[0])
	return strconv.FormatUint(uint64(val), 2), nil
}

func toOctFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("to_oct requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	val := valueInt(args[0])
	if val < 0 {
		return strconv.FormatUint(uint64(uint32(val)), 8), nil
	}
	return strconv.FormatInt(val, 8), nil
}

func toOctBigintFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("to_oct requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	val := valueInt(args[0])
	return strconv.FormatUint(uint64(val), 8), nil
}

func toHexFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("to_hex requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	val := valueInt(args[0])
	if val < 0 {
		return strconv.FormatUint(uint64(uint32(val)), 16), nil
	}
	return strconv.FormatInt(val, 16), nil
}

func toHexBigintFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("to_hex requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	val := valueInt(args[0])
	return strconv.FormatUint(uint64(val), 16), nil
}

func sha224Func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sha224 requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	sum := sha256.Sum224(data)
	return sum[:], nil
}

func sha256Func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sha256 requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	sum := sha256.Sum256(data)
	return sum[:], nil
}

func sha384Func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sha384 requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	sum := sha512.Sum384(data)
	return sum[:], nil
}

func sha512Func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sha512 requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	sum := sha512.Sum512(data)
	return sum[:], nil
}

func crc32Func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("crc32 requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	checksum := crc32.ChecksumIEEE(data)
	return int64(checksum), nil
}

func crc32cFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("crc32c requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	table := crc32.MakeTable(crc32.Castagnoli)
	checksum := crc32.Checksum(data, table)
	return int64(checksum), nil
}

func getBitFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("get_bit requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	bitIdx := valueInt(args[1])
	numBits := int64(len(data)) * 8
	if bitIdx < 0 || bitIdx >= numBits {
		return nil, fmt.Errorf("index %d out of valid range, 0..%d", bitIdx, numBits-1)
	}
	byteIdx := bitIdx / 8
	bitOffset := bitIdx % 8
	bit := (data[byteIdx] >> bitOffset) & 1
	return int64(bit), nil
}

func setBitFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("set_bit requires 3 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = make([]byte, len(v))
		copy(data, v)
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	bitIdx := valueInt(args[1])
	newVal := valueInt(args[2])
	numBits := int64(len(data)) * 8
	if bitIdx < 0 || bitIdx >= numBits {
		return nil, fmt.Errorf("index %d out of valid range, 0..%d", bitIdx, numBits-1)
	}
	byteIdx := bitIdx / 8
	bitOffset := bitIdx % 8
	if newVal != 0 {
		data[byteIdx] |= (1 << bitOffset)
	} else {
		data[byteIdx] &= ^(1 << bitOffset)
	}
	return data, nil
}

func getByteFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("get_byte requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	byteIdx := valueInt(args[1])
	numBytes := int64(len(data))
	if byteIdx < 0 || byteIdx >= numBytes {
		return nil, fmt.Errorf("index %d out of valid range, 0..%d", byteIdx, numBytes-1)
	}
	return int64(data[byteIdx]), nil
}

func setByteFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("set_byte requires 3 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = make([]byte, len(v))
		copy(data, v)
	case string:
		data = []byte(v)
	default:
		data = []byte(fmt.Sprint(v))
	}
	byteIdx := valueInt(args[1])
	newVal := valueInt(args[2])
	numBytes := int64(len(data))
	if byteIdx < 0 || byteIdx >= numBytes {
		return nil, fmt.Errorf("index %d out of valid range, 0..%d", byteIdx, numBytes-1)
	}
	data[byteIdx] = byte(newVal)
	return data, nil
}

func initcapFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("initcap requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	str := valueString(args[0])
	var b strings.Builder
	inWord := false
	for _, r := range str {
		isAlphanumeric := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if isAlphanumeric {
			if !inWord {
				b.WriteRune(unicode.ToUpper(r))
				inWord = true
			} else {
				b.WriteRune(unicode.ToLower(r))
			}
		} else {
			b.WriteRune(r)
			inWord = false
		}
	}
	return b.String(), nil
}

func translateFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("translate requires 3 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}
	str := valueString(args[0])
	from := valueString(args[1])
	to := valueString(args[2])

	fromRunes := []rune(from)
	toRunes := []rune(to)

	mapping := make(map[rune]rune)
	deleted := make(map[rune]bool)

	for i, fr := range fromRunes {
		if _, exists := mapping[fr]; exists {
			continue
		}
		if _, exists := deleted[fr]; exists {
			continue
		}
		if i < len(toRunes) {
			mapping[fr] = toRunes[i]
		} else {
			deleted[fr] = true
		}
	}

	var b strings.Builder
	for _, r := range str {
		if deleted[r] {
			continue
		}
		if tr, ok := mapping[r]; ok {
			b.WriteRune(tr)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String(), nil
}

func bitCountFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("bit_count requires 1 argument")
	}
	if args[0] == nil {
		return nil, nil
	}
	var data []byte
	switch v := args[0].(type) {
	case []byte:
		data = v
	case int64:
		return int64(bits.OnesCount64(uint64(v))), nil
	case string:
		data = []byte(v)
	default:
		if val, err := strconv.ParseInt(fmt.Sprint(v), 10, 64); err == nil {
			return int64(bits.OnesCount64(uint64(val))), nil
		}
		data = []byte(fmt.Sprint(v))
	}
	count := 0
	for _, b := range data {
		count += bits.OnesCount8(b)
	}
	return int64(count), nil
}

func ltrimFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return nil, fmt.Errorf("ltrim requires 1 or 2 arguments")
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
	return strings.TrimLeft(valueString(args[0]), cutset), nil
}

func rtrimFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return nil, fmt.Errorf("rtrim requires 1 or 2 arguments")
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
	return strings.TrimRight(valueString(args[0]), cutset), nil
}

func minipsqlErrorFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("minipsql_error requires 3 arguments")
	}
	msg := valueString(args[0])
	code := valueString(args[1])
	hint := valueString(args[2])
	return nil, &compat.PGError{
		Code:    code,
		Message: msg,
		Hint:    hint,
	}
}

func minipsqlSimilarSubstringFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("minipsql_similar_substring requires 3 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	val := valueString(args[0])
	pat := valueString(args[1])
	escStr := "\\"
	if args[2] != nil {
		escStr = valueString(args[2])
	}
	if len(escStr) != 1 {
		return nil, &compat.PGError{Code: "22025", Message: "invalid escape sequence"}
	}
	esc := rune(escStr[0])

	sep := string(esc) + `"`
	parts := strings.Split(pat, sep)
	if len(parts) > 3 {
		return nil, &compat.PGError{Code: "22025", Message: "more than two part separators"}
	}

	var r1, r2, r3 string
	var err error
	if len(parts) == 1 {
		r2, err = similarPartToRegex(parts[0], esc, false)
	} else if len(parts) == 2 {
		r1, err = similarPartToRegex(parts[0], esc, true)
		if err == nil {
			r2, err = similarPartToRegex(parts[1], esc, false)
		}
	} else {
		r1, err = similarPartToRegex(parts[0], esc, true)
		if err == nil {
			r2, err = similarPartToRegex(parts[1], esc, false)
		}
		if err == nil {
			r3, err = similarPartToRegex(parts[2], esc, true)
		}
	}

	if err != nil {
		return nil, &compat.PGError{Code: "22025", Message: "invalid escape sequence"}
	}

	fullRegexStr := "^(?P<p1>" + r1 + ")(?P<p2>" + r2 + ")(?P<p3>" + r3 + ")$"
	re, err := regexp.Compile(fullRegexStr)
	if err != nil {
		return nil, &compat.PGError{Code: "22025", Message: "invalid regular expression"}
	}

	match := re.FindStringSubmatch(val)
	if match == nil {
		return nil, nil
	}

	p2Idx := re.SubexpIndex("p2")
	if p2Idx >= 0 && p2Idx < len(match) {
		return match[p2Idx], nil
	}
	return nil, nil
}

func similarPartToRegex(s string, esc rune, nonGreedy bool) (string, error) {
	var b strings.Builder
	runes := []rune(s)
	n := len(runes)
	for i := 0; i < n; i++ {
		r := runes[i]
		if r == esc {
			if i+1 < n {
				next := runes[i+1]
				if next == '_' || next == '%' || next == esc || next == '"' {
					b.WriteByte('\\')
					b.WriteRune(next)
					i++
				} else {
					return "", fmt.Errorf("invalid escape sequence")
				}
			} else {
				return "", fmt.Errorf("invalid escape sequence")
			}
		} else {
			switch r {
			case '%':
				if nonGreedy {
					b.WriteString(".*?")
				} else {
					b.WriteString(".*")
				}
			case '_':
				b.WriteString(".")
			case '*', '+', '?':
				b.WriteRune(r)
				if nonGreedy {
					b.WriteByte('?')
				}
			case '(', ')', '|', '{', '}', '[', ']':
				b.WriteRune(r)
			case '.', '^', '$', '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			default:
				if strings.ContainsRune(`+*?()|[]{}^$.`, r) {
					b.WriteByte('\\')
				}
				b.WriteRune(r)
			}
		}
	}
	return b.String(), nil
}

func minipsqlSimilarToFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("minipsql_similar_to requires 3 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	if args[2] == nil {
		return nil, nil
	}
	text := valueString(args[0])
	pat := valueString(args[1])
	escStr := valueString(args[2])

	var esc rune
	var useEsc bool
	if escStr == "" {
		useEsc = false
	} else if utf8.RuneCountInString(escStr) == 1 {
		useEsc = true
		esc, _ = utf8.DecodeRuneInString(escStr)
	} else {
		return nil, &compat.PGError{Code: "22025", Message: "invalid escape string"}
	}

	var regexPat string
	var err error
	if useEsc {
		regexPat, err = similarPartToRegex(pat, esc, false)
	} else {
		regexPat, err = similarPartToRegex(pat, 0, false)
	}

	if err != nil {
		return nil, &compat.PGError{Code: "22025", Message: "invalid escape sequence"}
	}

	re, err := regexp.Compile("^" + regexPat + "$")
	if err != nil {
		return nil, &compat.PGError{Code: "22025", Message: "invalid regular expression"}
	}

	if re.MatchString(text) {
		return int64(1), nil
	}
	return int64(0), nil
}

func booleqFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("booleq requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	// Convert both to boolean/integer (0 or 1)
	v1 := valueInt(args[0]) != 0
	v2 := valueInt(args[1]) != 0
	if v1 == v2 {
		return int64(1), nil
	}
	return int64(0), nil
}

func boolneFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("boolne requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	v1 := valueInt(args[0]) != 0
	v2 := valueInt(args[1]) != 0
	if v1 != v2 {
		return int64(1), nil
	}
	return int64(0), nil
}

func gcdFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("gcd requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	a := valueInt(args[0])
	b := valueInt(args[1])

	isInt32 := (a >= -2147483648 && a <= 2147483647) && (b >= -2147483648 && b <= 2147483647)

	var x, y big.Int
	x.SetInt64(a)
	y.SetInt64(b)
	x.Abs(&x)
	y.Abs(&y)
	
	var z big.Int
	z.GCD(nil, nil, &x, &y)

	if isInt32 {
		if !z.IsInt64() || z.Int64() > 2147483647 {
			return nil, &compat.PGError{Code: "22003", Message: "integer out of range"}
		}
	} else {
		if !z.IsInt64() {
			return nil, &compat.PGError{Code: "22003", Message: "bigint out of range"}
		}
	}

	return z.Int64(), nil
}

func lcmFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("lcm requires 2 arguments")
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	a := valueInt(args[0])
	b := valueInt(args[1])

	isInt32 := (a >= -2147483648 && a <= 2147483647) && (b >= -2147483648 && b <= 2147483647)

	var x, y big.Int
	x.SetInt64(a)
	y.SetInt64(b)
	x.Abs(&x)
	y.Abs(&y)

	if x.Sign() == 0 || y.Sign() == 0 {
		return int64(0), nil
	}

	var g big.Int
	g.GCD(nil, nil, &x, &y)

	var mul big.Int
	mul.Mul(&x, &y)

	var z big.Int
	z.Div(&mul, &g)

	if isInt32 {
		if !z.IsInt64() || z.Int64() > 2147483647 {
			return nil, &compat.PGError{Code: "22003", Message: "integer out of range"}
		}
	} else {
		if !z.IsInt64() {
			return nil, &compat.PGError{Code: "22003", Message: "bigint out of range"}
		}
	}

	return z.Int64(), nil
}















