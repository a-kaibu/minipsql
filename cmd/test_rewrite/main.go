package main

import (
	"fmt"
	"github.com/yashikota/minipsql/internal/compat"
)

func main() {
	tests := []string{
		"SELECT aggfnoid FROM pg_aggregate WHERE aggfnoid = 'myavg'::REGPROC",
		"SELECT oid FROM pg_type WHERE typname = 'text'::regtype",
	}
	for _, t := range tests {
		result := compat.Rewrite(t)
		fmt.Printf("IN:  %s\nOUT: %s\n\n", t, result)
	}
}
