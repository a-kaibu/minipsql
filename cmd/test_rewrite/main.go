package main

import (
	"fmt"
	"github.com/yashikota/minipsql/internal/compat"
)

func main() {
	tests := []string{
		`SELECT x, y FROM (SELECT 10*cosd(a), 10*sind(a) FROM generate_series(0, 360, 90) AS t(a)) AS t(x,y)`,
		`SELECT * FROM (SELECT 1 AS a, 2 AS b) AS sub(x, y)`,
	}
	for _, t := range tests {
		result := compat.Rewrite(t)
		fmt.Printf("IN:  %s\nOUT: %s\n\n", t, result)
	}
}
