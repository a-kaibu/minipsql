package main

import (
	"fmt"
	"github.com/yashikota/minipsql/internal/compat"
)

func main() {
	tests := []string{
		`select * from int8_tbl limit (case when random() < 0.5 then null::bigint end)`,
		`select * from int8_tbl offset (case when random() < 0.5 then null::bigint end)`,
		`select unique1, unique2, generate_series(1,10) from tenk1 order by unique2 limit 7`,
	}
	for _, t := range tests {
		result := compat.Rewrite(t)
		fmt.Printf("IN:  %s\nOUT: %s\n\n", t, result)
	}
}
