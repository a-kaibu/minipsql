package copy

import (
	"testing"
)

func TestParseStdin(t *testing.T) {
	s, err := Parse("COPY foo FROM STDIN")
	if err != nil || s == nil {
		t.Fatalf("unexpected: %v, %v", s, err)
	}
	if s.TableName != "foo" || s.Direction != "from" || s.Source != "stdin" {
		t.Errorf("got %+v", s)
	}
	if s.Format != "text" || s.Delimiter != '\t' || s.NullString != `\N` {
		t.Errorf("defaults wrong: %+v", s)
	}
}

func TestParseColumns(t *testing.T) {
	s, err := Parse("COPY tbl (a, b, c) FROM STDIN")
	if err != nil || s == nil {
		t.Fatalf("%v %v", s, err)
	}
	if len(s.Columns) != 3 || s.Columns[0] != "a" || s.Columns[2] != "c" {
		t.Errorf("columns: %v", s.Columns)
	}
}

func TestParseCSV(t *testing.T) {
	s, err := Parse("COPY tbl FROM STDIN CSV")
	if err != nil || s == nil {
		t.Fatalf("%v %v", s, err)
	}
	if s.Format != "csv" || s.Delimiter != ',' || s.NullString != "" {
		t.Errorf("csv defaults wrong: %+v", s)
	}
}

func TestParseNullOption(t *testing.T) {
	s, err := Parse("COPY tbl FROM STDIN NULL 'null'")
	if err != nil || s == nil {
		t.Fatalf("%v %v", s, err)
	}
	if s.NullString != "null" {
		t.Errorf("null string: %q", s.NullString)
	}
}

func TestParseFile(t *testing.T) {
	s, err := Parse("COPY tbl FROM '/tmp/data.tsv'")
	if err != nil || s == nil {
		t.Fatalf("%v %v", s, err)
	}
	if s.Source != "/tmp/data.tsv" {
		t.Errorf("source: %q", s.Source)
	}
}

func TestParseNonCopy(t *testing.T) {
	s, err := Parse("SELECT 1")
	if err != nil || s != nil {
		t.Errorf("non-COPY should return nil: %v %v", s, err)
	}
}

func TestParseTextRow(t *testing.T) {
	fields, eof, err := ParseTextRow("a\tb\tc", '\t', `\N`)
	if err != nil || eof {
		t.Fatalf("%v %v", eof, err)
	}
	if len(fields) != 3 || fields[0] != "a" || fields[2] != "c" {
		t.Errorf("fields: %v", fields)
	}
}

func TestParseTextRowNull(t *testing.T) {
	fields, eof, err := ParseTextRow(`a\Nb`, '\t', `\N`)
	if err != nil || eof {
		t.Fatalf("%v %v", eof, err)
	}
	if len(fields) != 1 || !IsNullMarker(fields[0]) {
		t.Errorf("expected null marker, got %v", fields)
	}
}

func TestParseTextRowEOF(t *testing.T) {
	_, eof, err := ParseTextRow(`\.`, '\t', `\N`)
	if err != nil || !eof {
		t.Errorf("expected eof=true, got eof=%v err=%v", eof, err)
	}
}

func TestParseCSVRow(t *testing.T) {
	fields, eof, err := ParseCSVRow(`"hello","world"`, ',', "", '"', '"')
	if err != nil || eof {
		t.Fatalf("%v %v", eof, err)
	}
	if len(fields) != 2 || fields[0] != "hello" || fields[1] != "world" {
		t.Errorf("fields: %v", fields)
	}
}

func TestParseSchemaQualified(t *testing.T) {
	s, err := Parse("COPY public.foo FROM STDIN")
	if err != nil || s == nil {
		t.Fatalf("%v %v", s, err)
	}
	if s.TableName != "foo" {
		t.Errorf("table name: %q", s.TableName)
	}
}
