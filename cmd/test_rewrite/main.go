package main

import (
	"fmt"
	"github.com/yashikota/minipsql/internal/compat"
)

func main() {
	tests := []string{
		"CREATE TABLE large_tuple_test (a int, b text) WITH (fillfactor = 10)",
		"INSERT INTO t (select 1, NULL)",
		"INSERT INTO large_tuple_test (select 2, repeat('a', 1000))",
		"CREATE TABLE withoid() WITH OIDS",
	}
	for _, s := range tests {
		out := compat.Rewrite(s)
		if out != s {
			fmt.Printf("IN:  %s\nOUT: %s\n\n", s, out)
		} else {
			fmt.Printf("UNCHANGED: %s\n\n", s)
		}
	}
}
