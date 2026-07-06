package compat

import (
	"reflect"
	"testing"
)

func TestSplitStatementsSkipsComments(t *testing.T) {
	sql := `
SELECT 'before';
/* outer
   /* nested */
*/
SELECT -- line comment
  'after';
/* final comment */
`
	got := SplitStatements(sql)
	want := []string{
		"SELECT 'before'",
		"SELECT \n  'after'",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitStatements() = %#v, want %#v", got, want)
	}
}

func TestSplitStatementsKeepsDollarQuotedBlockTogether(t *testing.T) {
	sql := `DO $doblock$
begin
  perform 1;
end
$doblock$;
SELECT 1;`
	got := SplitStatements(sql)
	want := []string{
		`DO $doblock$
begin
  perform 1;
end
$doblock$`,
		"SELECT 1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitStatements() = %#v, want %#v", got, want)
	}
}

func TestRewritePositionIn(t *testing.T) {
	sql := "SELECT POSITION(convert_from('\\xbcf6', 'EUC_KR') IN\n  convert_from('\\xb0fa\\xbcf6', 'EUC_KR'))"
	want := `SELECT instr(convert_from('\xb0fa\xbcf6', 'EUC_KR'), convert_from('\xbcf6', 'EUC_KR'))`
	if got := Rewrite(sql); got != want {
		t.Fatalf("Rewrite() = %q, want %q", got, want)
	}
}

func TestRewriteCharLiteral(t *testing.T) {
	sql := "SELECT char 'c' = char 'c'"
	want := "SELECT 'c' = 'c'"
	if got := Rewrite(sql); got != want {
		t.Fatalf("Rewrite() = %q, want %q", got, want)
	}
}

func TestRewriteQuotedCharCast(t *testing.T) {
	sql := `SELECT 'a'::"char"::text`
	want := "SELECT CAST(CAST('a' AS text) AS text)"
	if got := Rewrite(sql); got != want {
		t.Fatalf("Rewrite() = %q, want %q", got, want)
	}
}
