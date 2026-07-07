package main

import (
    "fmt"
    "os"
    "github.com/yashikota/minipsql/internal/compat"
)

func main() {
    tests := []string{
        `SELECT (SELECT n FROM (VALUES (1)) AS x, (SELECT n FROM generate_series(1,10) AS n ORDER BY n LIMIT 1 OFFSET s-1) AS y) AS z FROM generate_series(1,10) AS s`,
    }
    if len(os.Args) > 1 {
        tests = os.Args[1:]
    }
    for _, sql := range tests {
        result := compat.Rewrite(sql)
        fmt.Printf("IN:  %s\nOUT: %s\n\n", sql, result)
    }
}
