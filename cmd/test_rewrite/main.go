package main

import (
	"fmt"
	"github.com/yashikota/minipsql/internal/compat"
)

func main() {
	tests := []string{
		"SELECT * FROM unnest(array['a', 'b', 'c']) as u(prop,ord)",
		"SELECT * FROM unnest(ARRAY['a', 'b', 'c']) WITH ORDINALITY as u(prop,ord)",
		"SELECT unnest(ARRAY[1,2,3]) as u",
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
