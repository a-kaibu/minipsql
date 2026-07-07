package main

import (
  "fmt"
  "github.com/yashikota/minipsql/internal/compat"
)

func main() {
  sql := "create materialized view parallel_mat_view as\n    select length(stringu1) from tenk1 group by length(stringu1)"
  fmt.Printf("IN:  %q\nOUT: %q\n", sql, compat.Rewrite(sql))
}
