package main

import (
	"fmt"
	"github.com/yashikota/minipsql/internal/compat"
)

func main() {
	tests := []string{
		"select g*10 from t",
		"select a*b from t",
		"select t.* from t",
		"select t.*, t.a*2 from t",
		"SELECT * FROM person* WHERE name='Jones'",
		"SELECT f(x*2) FROM t",
		"select g, point(g*10, g*10) from generate_series(1, 10) g",
	}
	for _, t := range tests {
		result := compat.Rewrite(t)
		fmt.Printf("IN:  %s\nOUT: %s\n\n", t, result)
	}
}
