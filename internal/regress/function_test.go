package regress_test

import "testing"

func TestStringFunctions(t *testing.T) {
	run(t, testGroup{
		Name: "string_functions",
		Cases: map[string]testCase{
			"length": {
				SQL:  "SELECT LENGTH('hello')",
				Want: [][]string{{"5"}},
			},
			"length_empty": {
				SQL:  "SELECT LENGTH('')",
				Want: [][]string{{"0"}},
			},
			"upper": {
				SQL:  "SELECT UPPER('hello')",
				Want: [][]string{{"HELLO"}},
			},
			"lower": {
				SQL:  "SELECT LOWER('HELLO')",
				Want: [][]string{{"hello"}},
			},
			"trim_both": {
				SQL:  "SELECT TRIM('  hello  ')",
				Want: [][]string{{"hello"}},
			},
			"ltrim": {
				SQL:  "SELECT LTRIM('  hello')",
				Want: [][]string{{"hello"}},
			},
			"rtrim": {
				SQL:  "SELECT RTRIM('hello  ')",
				Want: [][]string{{"hello"}},
			},
			"substr_two_args": {
				SQL:  "SELECT SUBSTR('hello', 2)",
				Want: [][]string{{"ello"}},
			},
			"substr_three_args": {
				SQL:  "SELECT SUBSTR('hello', 2, 3)",
				Want: [][]string{{"ell"}},
			},
			"replace": {
				SQL:  "SELECT REPLACE('hello world', 'world', 'go')",
				Want: [][]string{{"hello go"}},
			},
		},
	})
}

func TestMathFunctions(t *testing.T) {
	run(t, testGroup{
		Name: "math_functions",
		Cases: map[string]testCase{
			"abs_positive": {
				SQL:  "SELECT ABS(-5)",
				Want: [][]string{{"5"}},
			},
			"abs_zero": {
				SQL:  "SELECT ABS(0)",
				Want: [][]string{{"0"}},
			},
			"max_scalar": {
				SQL:  "SELECT MAX(1, 5, 3)",
				Want: [][]string{{"5"}},
				Skip: "multi-arg MAX is not standard SQL aggregate",
			},
			"min_scalar": {
				SQL:  "SELECT MIN(5, 1, 3)",
				Want: [][]string{{"1"}},
				Skip: "multi-arg MIN is not standard SQL aggregate",
			},
		},
	})
}

func TestTypeCast(t *testing.T) {
	run(t, testGroup{
		Name: "cast",
		Cases: map[string]testCase{
			"int_to_text": {
				SQL:  "SELECT CAST(42 AS TEXT)",
				Want: [][]string{{"42"}},
			},
			"text_to_int": {
				SQL:  "SELECT CAST('123' AS INTEGER)",
				Want: [][]string{{"123"}},
			},
			"float_to_int": {
				SQL:  "SELECT CAST(3.7 AS INTEGER)",
				Want: [][]string{{"3"}},
			},
			"pg_cast_syntax": {
				SQL:  "SELECT '42'::INTEGER",
				Want: [][]string{{"42"}},
				Skip: "PostgreSQL :: cast syntax not implemented",
			},
		},
	})
}
