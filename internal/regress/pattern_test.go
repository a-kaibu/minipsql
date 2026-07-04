package regress_test

import "testing"

func TestRegexOperator(t *testing.T) {
	run(t, testGroup{
		Name: "REGEX_OPERATOR",
		Setup: []string{
			"CREATE TABLE words (id INTEGER PRIMARY KEY, word TEXT)",
			"INSERT INTO words VALUES (1, 'hello'), (2, 'world'), (3, 'Hello'), (4, 'WORLD'), (5, 'help')",
		},
		Cases: map[string]testCase{
			"tilde_match": {
				SQL:  "SELECT word FROM words WHERE word ~ 'hel' ORDER BY id",
				Skip: "not yet implemented: regex operator ~",
				Want: [][]string{{"hello"}, {"help"}},
			},
			"tilde_anchored": {
				SQL:  "SELECT word FROM words WHERE word ~ '^hel' ORDER BY id",
				Skip: "not yet implemented: regex operator ~",
				Want: [][]string{{"hello"}, {"help"}},
			},
			"tilde_star_icase": {
				SQL:  "SELECT word FROM words WHERE word ~* 'hello' ORDER BY id",
				Skip: "not yet implemented: regex operator ~*",
				Want: [][]string{{"hello"}, {"Hello"}},
			},
			"not_tilde": {
				SQL:  "SELECT word FROM words WHERE word !~ 'hel' ORDER BY id",
				Skip: "not yet implemented: regex operator !~",
				Want: [][]string{{"world"}, {"Hello"}, {"WORLD"}},
			},
		},
	})
}

func TestSimilarTo(t *testing.T) {
	run(t, testGroup{
		Name: "SIMILAR_TO",
		Setup: []string{
			"CREATE TABLE texts (id INTEGER PRIMARY KEY, val TEXT)",
			"INSERT INTO texts VALUES (1, 'abc'), (2, 'adc'), (3, 'aec'), (4, 'xyz')",
		},
		Cases: map[string]testCase{
			"similar_to_basic": {
				SQL:  "SELECT val FROM texts WHERE val SIMILAR TO 'a_c' ORDER BY id",
				Skip: "not yet implemented: SIMILAR TO",
				Want: [][]string{{"abc"}, {"adc"}, {"aec"}},
			},
			"similar_to_alternation": {
				SQL:  "SELECT val FROM texts WHERE val SIMILAR TO '(abc|xyz)' ORDER BY id",
				Skip: "not yet implemented: SIMILAR TO",
				Want: [][]string{{"abc"}, {"xyz"}},
			},
		},
	})
}
