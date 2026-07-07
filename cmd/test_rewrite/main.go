package main

import (
    "fmt"
    "os"
    "github.com/yashikota/minipsql/internal/compat"
)

func main() {
    tests := []string{
        "SELECT 1 FROM generate_series(1,3) UNION ALL SELECT 2",
        "INSERT INTO test_mode SELECT 1 FROM generate_series(1,1000) UNION ALL SELECT 2",
    }
    if len(os.Args) > 1 {
        tests = os.Args[1:]
    }
    for _, sql := range tests {
        result := compat.Rewrite(sql)
        fmt.Printf("IN:  %s\nOUT: %s\n\n", sql, result)
    }
}
