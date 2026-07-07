package main

import (
	"fmt"
	"github.com/yashikota/minipsql/internal/compat"
)

func main() {
	tests := []string{
		`WHERE substring(ctid::text FROM ',(\d+)\)')::integer > 10 OR substring(ctid::text FROM '\((\d+),')::integer > 2`,
	}
	for _, t := range tests {
		result := compat.Rewrite(t)
		fmt.Printf("IN:  %s\nOUT: %s\n\n", t, result)
	}
}
