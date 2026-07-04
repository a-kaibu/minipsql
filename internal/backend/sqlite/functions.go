package sqlite

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"sync"

	"modernc.org/sqlite"
)

var regexCache sync.Map

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("regexp_match", 3, regexpMatchFunc)
}

func regexpMatchFunc(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("regexp_match requires 3 arguments")
	}
	if args[0] == nil || args[1] == nil || args[2] == nil {
		return nil, nil
	}

	toString := func(v driver.Value) string {
		switch val := v.(type) {
		case string:
			return val
		case []byte:
			return string(val)
		default:
			return fmt.Sprintf("%v", val)
		}
	}

	pattern := toString(args[0])
	text := toString(args[1])
	flags := toString(args[2])

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
