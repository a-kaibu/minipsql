package backend

import "context"

type Backend interface {
	Query(ctx context.Context, sql string, args []Value) (*Result, error)
	Exec(ctx context.Context, sql string, args []Value) (*CommandTag, error)
	Begin(ctx context.Context) error
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	Close(ctx context.Context) error
	BulkInsert(ctx context.Context, table string, columns []string, rows [][]Value) (int64, error)
	TableColumns(ctx context.Context, table string) ([]string, error)
}

type Result struct {
	Columns []Column
	Rows    [][]Value
}

type Column struct {
	Name    string
	TypeOID uint32
	Format  int16
}

type ValueKind int

const (
	ValueNull ValueKind = iota
	ValueBool
	ValueInt
	ValueFloat
	ValueText
	ValueBytes
)

type Value struct {
	Kind  ValueKind
	Value any
}

type CommandTag struct {
	Tag string
}
