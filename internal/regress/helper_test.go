package regress_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/backend/sqlite"
	"github.com/yashikota/minipsql/internal/session"
)

type testCase struct {
	SQL     string
	Want    [][]string
	WantErr bool
	WantTag string
	Skip    string
}

type testGroup struct {
	Name  string
	Setup []string
	Cases map[string]testCase
}

func newSession(t *testing.T) (*session.Session, context.Context) {
	t.Helper()
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close(ctx) })
	return session.New(1, 1, "testdb", "testuser", b), ctx
}

func run(t *testing.T, g testGroup) {
	t.Helper()
	t.Run(g.Name, func(t *testing.T) {
		s, ctx := newSession(t)

		for _, sql := range g.Setup {
			_, _, err := s.Execute(ctx, sql, nil)
			if err != nil {
				t.Fatalf("setup %q: %v", sql, err)
			}
		}

		for name, tc := range g.Cases {
			t.Run(name, func(t *testing.T) {
				if tc.Skip != "" {
					t.Skip(tc.Skip)
				}

				result, tag, err := s.Execute(ctx, tc.SQL, nil)

				if tc.WantErr {
					if err == nil {
						t.Fatal("expected error, got nil")
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if tc.WantTag != "" {
					if tag == nil {
						t.Fatalf("expected tag %q, got nil", tc.WantTag)
					}
					if tag.Tag != tc.WantTag {
						t.Fatalf("tag = %q, want %q", tag.Tag, tc.WantTag)
					}
					return
				}

				if tc.Want != nil {
					assertRows(t, result, tc.Want)
				}
			})
		}
	})
}

func assertRows(t *testing.T, result *backend.Result, want [][]string) {
	t.Helper()
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if len(result.Rows) != len(want) {
		t.Fatalf("got %d rows, want %d\nrows: %v", len(result.Rows), len(want), formatRows(result))
	}
	for i, wantRow := range want {
		if len(result.Rows[i]) != len(wantRow) {
			t.Fatalf("row %d: got %d cols, want %d", i, len(result.Rows[i]), len(wantRow))
		}
		for j, wantVal := range wantRow {
			got := fmt.Sprint(result.Rows[i][j].Value)
			if got != wantVal {
				t.Errorf("row %d col %d: got %q, want %q", i, j, got, wantVal)
			}
		}
	}
}

func formatRows(result *backend.Result) string {
	if result == nil {
		return "<nil>"
	}
	var s string
	for i, row := range result.Rows {
		if i > 0 {
			s += ", "
		}
		s += "["
		for j, v := range row {
			if j > 0 {
				s += " "
			}
			s += fmt.Sprint(v.Value)
		}
		s += "]"
	}
	return s
}
