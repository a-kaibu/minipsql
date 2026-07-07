package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestParseCircle(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"<(5,1),3>", true},
		{"((1,2),100)", true},
		{" 1 , 3 , 5 ", true},
		{" ( ( 1 , 2 ) , 3 ) ", true},
		{" ( 100 , 200 ) , 10 ", true},
		{" < ( 100 , 1 ) , 115 > ", true},
		{"<(3,5),0>", true},
		{"<(3,5),NaN>", true},
		{"<(-100,0),-100>", false},
		{"<(100,200),10", false},
		{"<(100,200),10> x", false},
		{"1abc,3,5", false},
		{"(3,(1,2),3)", false},
	}
	for _, tc := range cases {
		x, y, r, ok := parseCircle(tc.in)
		if ok != tc.ok {
			t.Errorf("parseCircle(%q) = (x=%g, y=%g, r=%g, ok=%v), want %v", tc.in, x, y, r, ok, tc.ok)
		}
	}
}

func TestRegexFunctions(t *testing.T) {
	ctx := context.Background()
	b, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close(ctx)

	sess := b.NewSession()
	defer sess.Close(ctx)

	cases := []struct {
		query    string
		expected string
	}{
		// 1. minipsql_regexp_match(pattern, source, flags)
		{"SELECT minipsql_regexp_match('h.*o', 'hello', '')", "1"},
		{"SELECT minipsql_regexp_match('h.*o', 'hello', 'i')", "1"},
		{"SELECT minipsql_regexp_match('world', 'hello', '')", "0"},

		// 2. regexp_match(source, pattern [, flags])
		{"SELECT regexp_match('hello', 'h.*o')", `{"hello"}`},
		{"SELECT regexp_match('foobarbequebaz', '(bar)(beque)')", `{"bar","beque"}`},
		{"SELECT regexp_match('hello', 'world')", ""}, // NULL

		// 3. regexp_like(source, pattern [, flags])
		{"SELECT regexp_like('hello', 'h.*o')", "1"},
		{"SELECT regexp_like('hello', 'WORLD')", "0"},
		{"SELECT regexp_like('hello', 'HELLO', 'i')", "1"},

		// 4. regexp_count(source, pattern [, position [, flags]])
		{"SELECT regexp_count('hello world hello', 'hello')", "2"},
		{"SELECT regexp_count('hello world hello', 'hello', 5)", "1"},
		{"SELECT regexp_count('hello world hello', 'HELLO', 1, 'i')", "2"},

		// 5. regexp_instr(source, pattern [, position [, occurrence [, return_option [, flags]]]])
		{"SELECT regexp_instr('hello world', 'world')", "7"},
		{"SELECT regexp_instr('hello world', 'o', 1, 2)", "8"},
		{"SELECT regexp_instr('hello world', 'o', 1, 1, 1)", "6"},

		// 6. regexp_replace(source, pattern, replacement [, position [, occurrence], flags])
		{"SELECT regexp_replace('hello world', 'world', 'there')", "hello there"},
		{"SELECT regexp_replace('hello world', 'o', 'x', 1, 0)", "hellx wxrld"},
		{"SELECT regexp_replace('hello world hello', 'hello', 'hi', 1, 2)", "hello world hi"},

		// 7. encode / decode
		{"SELECT encode('hello', 'hex')", "68656c6c6f"},
		{"SELECT encode('hello', 'base64')", "aGVsbG8="},
		{"SELECT encode('hello\n', 'escape')", "hello\\012"},
		{"SELECT CAST(decode('68656c6c6f', 'hex') AS text)", "hello"},
		{"SELECT CAST(decode('aGVsbG8=', 'base64') AS text)", "hello"},
		{"SELECT CAST(decode('hello\\\\\\012', 'escape') AS text)", "hello\\\n"},

		// 8. strpos
		{"SELECT strpos('high', 'ig')", "2"},
		{"SELECT strpos('high', 'x')", "0"},
		{"SELECT strpos('high', '')", "1"},

		// 9. minipsql_overlay
		{"SELECT minipsql_overlay('abcdef', '45', 4)", "abc45f"},
		{"SELECT minipsql_overlay('yabadoo', 'daba', 5, 0)", "yabadabadoo"},

		// 10. split_part
		{"SELECT split_part('abc~@~def~@~ghi', '~@~', 2)", "def"},
		{"SELECT split_part('abc~@~def~@~ghi', '~@~', 4)", ""},

		// 11. sha2
		{"SELECT encode(sha224(''), 'hex')", "d14a028c2a3a2bc9476102bb288234c415a2b01f828ea62ac5b3e42f"},
		{"SELECT encode(sha256(''), 'hex')", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"SELECT encode(sha384(''), 'hex')", "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b"},
		{"SELECT encode(sha512(''), 'hex')", "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"},

		// 12. crc
		{"SELECT crc32('The quick brown fox jumps over the lazy dog.')", "1368401385"},
		{"SELECT crc32c('The quick brown fox jumps over the lazy dog.')", "419469235"},
		{"SELECT crc32('')", "0"},
		{"SELECT crc32c('')", "0"},

		// 13. base32hex / base64url
		{"SELECT encode(decode('112233', 'hex'), 'base32hex')", "24H36==="},
		{"SELECT encode(decode('24H36===', 'base32hex'), 'hex')", "112233"},
		{"SELECT encode(decode('24h36===', 'base32hex'), 'hex')", "112233"},
		{"SELECT encode(decode('24h36', 'base32hex'), 'hex')", "112233"},
		{"SELECT encode(decode('112233', 'hex'), 'base64url')", "ESIz"},
		{"SELECT encode(decode('ESIz', 'base64url'), 'hex')", "112233"},

		// 14. get_bit, set_bit, get_byte, set_byte
		{"SELECT get_bit(decode('1234567890abcdef00', 'hex'), 43)", "1"},
		{"SELECT encode(set_bit(decode('1234567890abcdef00', 'hex'), 43, 0), 'hex')", "1234567890a3cdef00"},
		{"SELECT get_byte(decode('1234567890abcdef00', 'hex'), 3)", "120"},
		{"SELECT encode(set_byte(decode('1234567890abcdef00', 'hex'), 7, 11), 'hex')", "1234567890abcd0b00"},

		// 15. initcap, lpad, rpad
		{"SELECT initcap('hello World!')", "Hello World!"},
		{"SELECT lpad('hi', 5)", "   hi"},
		{"SELECT rpad('hi', 5)", "hi   "},

		// 16. translate, bit_count
		{"SELECT translate('12345', '14', 'ax')", "a23x5"},
		{"SELECT translate('12345', '134', 'a')", "a25"},
		{"SELECT bit_count(decode('1234567890', 'hex'))", "15"},
	}

	for _, tc := range cases {
		res, err := sess.Query(ctx, tc.query, nil)
		if err != nil {
			t.Errorf("Query %q failed: %v", tc.query, err)
			continue
		}
		var got string
		if len(res.Rows) > 0 && len(res.Rows[0]) > 0 && res.Rows[0][0].Value != nil {
			got = fmt.Sprintf("%v", res.Rows[0][0].Value)
		}
		if got != tc.expected {
			t.Errorf("Query %q = %q, want %q", tc.query, got, tc.expected)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	ctx := context.Background()
	b, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close(ctx)

	sess := b.NewSession()
	defer sess.Close(ctx)

	cases := []struct {
		query       string
		expectedErr string
	}{
		{"SELECT decode('2=', 'base32hex')", `unexpected "=" while decoding base32hex sequence`},
		{"SELECT decode('=', 'base32hex')", `unexpected "=" while decoding base32hex sequence`},
		{"SELECT decode('W', 'base32hex')", `invalid symbol "W" found while decoding base32hex sequence`},
		{"SELECT decode('24H36H0=24', 'base32hex')", `invalid symbol "2" found while decoding base32hex sequence`},
		{"SELECT decode('QQ@=', 'base64url')", `invalid symbol "@" found while decoding base64url sequence`},
		{"SELECT decode('QQIDQ', 'base64url')", `invalid base64url end sequence`},
		{"SELECT decode('=QQQ', 'base64url')", `unexpected "=" while decoding base64url sequence`},
		{"SELECT get_bit(decode('1234567890abcdef00', 'hex'), 99)", "index 99 out of valid range, 0..71"},
		{"SELECT set_bit(decode('1234567890abcdef00', 'hex'), 99, 0)", "index 99 out of valid range, 0..71"},
		{"SELECT get_byte(decode('1234567890abcdef00', 'hex'), 99)", "index 99 out of valid range, 0..8"},
		{"SELECT set_byte(decode('1234567890abcdef00', 'hex'), 99, 11)", "index 99 out of valid range, 0..8"},
	}

	for _, tc := range cases {
		_, err := sess.Query(ctx, tc.query, nil)
		if err == nil {
			t.Errorf("Query %q succeeded, expected error", tc.query)
			continue
		}
		if !strings.Contains(err.Error(), tc.expectedErr) {
			t.Errorf("Query %q error = %q, want containing %q", tc.query, err.Error(), tc.expectedErr)
		}
	}
}

