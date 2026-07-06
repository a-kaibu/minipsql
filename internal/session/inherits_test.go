package session

import (
	"context"
	"strings"
	"testing"

	"github.com/yashikota/minipsql/internal/backend/sqlite"
)

func TestRewriteInherits(t *testing.T) {
	ctx := context.Background()
	b, err := sqlite.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close(ctx)

	s := New(1, 12345, "default", "mini", b, nil)

	// Create parent tables
	_, err = s.Backend.Exec(ctx, "CREATE TABLE person (name text, age integer)", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Backend.Exec(ctx, "CREATE TABLE student (gpa real)", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Test case 1: Single inheritance
	sql1 := "CREATE TABLE emp (salary integer) INHERITS (person);"
	got1, err := s.rewriteInherits(ctx, sql1)
	if err != nil {
		t.Fatal(err)
	}
	want1 := "create table emp (name text, age integer, salary integer)"
	if strings.ToLower(strings.TrimSuffix(got1, ";")) != want1 {
		t.Errorf("rewriteInherits() = %q, want %q", got1, want1)
	}

	// Test case 2: Multiple inheritance
	sql2 := "CREATE TABLE stud_emp (percent integer) INHERITS (person, student);"
	got2, err := s.rewriteInherits(ctx, sql2)
	if err != nil {
		t.Fatal(err)
	}
	want2 := "create table stud_emp (name text, age integer, gpa real, percent integer)"
	if strings.ToLower(strings.TrimSuffix(got2, ";")) != want2 {
		t.Errorf("rewriteInherits() = %q, want %q", got2, want2)
	}

	// Test case 3: Empty child columns list
	sql3 := "CREATE TABLE ihighway () INHERITS (person);"
	got3, err := s.rewriteInherits(ctx, sql3)
	if err != nil {
		t.Fatal(err)
	}
	want3 := "create table ihighway (name text, age integer)"
	if strings.ToLower(strings.TrimSuffix(got3, ";")) != want3 {
		t.Errorf("rewriteInherits() = %q, want %q", got3, want3)
	}
}
