package sqlite

import (
	"database/sql/driver"
	"fmt"
	"regexp"

	"modernc.org/sqlite"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("regexp_match", 3, regexpMatchFunc)
}

// regexp_match(pattern, text, flags)
// flags: 'i' for case-insensitive, '' for case-sensitive
func regexpMatchFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("regexp_match requires 3 arguments")
	}
	pattern, ok1 := args[0].(string)
	text, ok2 := args[1].(string)
	flags, ok3 := args[2].(string)
	if !ok1 || !ok2 || !ok3 {
		return int64(0), nil
	}
	if flags == "i" {
		pattern = "(?i)" + pattern
	}
	matched, err := regexp.MatchString(pattern, text)
	if err != nil {
		return int64(0), nil
	}
	if matched {
		return int64(1), nil
	}
	return int64(0), nil
}
