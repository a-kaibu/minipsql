package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/compat"
	_ "modernc.org/sqlite"
)

type Backend struct {
	db *sql.DB
	tx *sql.Tx
}

func New(ctx context.Context) (*Backend, error) {
	db, err := sql.Open("sqlite", "file:minipsql?mode=memory&cache=private")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Backend{db: db}, nil
}

func (b *Backend) Query(ctx context.Context, query string, args []backend.Value) (*backend.Result, error) {
	rows, err := b.queryer().QueryContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, _ := rows.ColumnTypes()
	cols := make([]backend.Column, len(names))
	for i, name := range names {
		dbType := ""
		if i < len(types) {
			dbType = types[i].DatabaseTypeName()
		}
		cols[i] = backend.Column{Name: name, TypeOID: compat.TypeOID(dbType), Format: 0}
	}

	var out [][]backend.Value
	for rows.Next() {
		raw := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]backend.Value, len(raw))
		for i, v := range raw {
			row[i] = valueFromDB(v)
			if cols[i].TypeOID == compat.OIDText && row[i].Kind != backend.ValueNull {
				cols[i].TypeOID = oidFromValue(row[i])
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &backend.Result{Columns: cols, Rows: out}, nil
}

func (b *Backend) Exec(ctx context.Context, query string, args []backend.Value) (*backend.CommandTag, error) {
	res, err := b.execer().ExecContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	return &backend.CommandTag{Tag: commandTag(query, n)}, nil
}

func (b *Backend) Begin(ctx context.Context) error {
	if b.tx != nil {
		return errors.New("transaction already in progress")
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	b.tx = tx
	return nil
}

func (b *Backend) Commit(ctx context.Context) error {
	if b.tx == nil {
		return errors.New("no transaction in progress")
	}
	err := b.tx.Commit()
	b.tx = nil
	return err
}

func (b *Backend) Rollback(ctx context.Context) error {
	if b.tx == nil {
		return errors.New("no transaction in progress")
	}
	err := b.tx.Rollback()
	b.tx = nil
	return err
}

func (b *Backend) Close(context.Context) error {
	return b.db.Close()
}

func (b *Backend) queryer() interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
} {
	if b.tx != nil {
		return b.tx
	}
	return b.db
}

func (b *Backend) execer() interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
} {
	if b.tx != nil {
		return b.tx
	}
	return b.db
}

func sqlArgs(vals []backend.Value) []any {
	args := make([]any, len(vals))
	for i, v := range vals {
		switch v.Kind {
		case backend.ValueNull:
			args[i] = nil
		default:
			args[i] = v.Value
		}
	}
	return args
}

func valueFromDB(v any) backend.Value {
	switch x := v.(type) {
	case nil:
		return backend.Value{Kind: backend.ValueNull}
	case bool:
		return backend.Value{Kind: backend.ValueBool, Value: x}
	case int64:
		return backend.Value{Kind: backend.ValueInt, Value: x}
	case int:
		return backend.Value{Kind: backend.ValueInt, Value: int64(x)}
	case float64:
		return backend.Value{Kind: backend.ValueFloat, Value: x}
	case []byte:
		return backend.Value{Kind: backend.ValueBytes, Value: x}
	case string:
		return backend.Value{Kind: backend.ValueText, Value: x}
	default:
		return backend.Value{Kind: backend.ValueText, Value: fmt.Sprint(x)}
	}
}

func oidFromValue(v backend.Value) uint32 {
	switch v.Kind {
	case backend.ValueBool:
		return compat.OIDBool
	case backend.ValueInt:
		return compat.OIDInt8
	case backend.ValueFloat:
		return compat.OIDFloat8
	case backend.ValueBytes:
		return compat.OIDBytea
	default:
		return compat.OIDText
	}
}

func commandTag(query string, affected int64) string {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return "OK"
	}
	switch strings.ToUpper(fields[0]) {
	case "INSERT":
		return "INSERT 0 " + strconv.FormatInt(affected, 10)
	case "UPDATE":
		return "UPDATE " + strconv.FormatInt(affected, 10)
	case "DELETE":
		return "DELETE " + strconv.FormatInt(affected, 10)
	case "CREATE":
		if len(fields) > 1 {
			return "CREATE " + strings.ToUpper(fields[1])
		}
		return "CREATE"
	case "DROP":
		if len(fields) > 1 {
			return "DROP " + strings.ToUpper(fields[1])
		}
		return "DROP"
	default:
		return strings.ToUpper(fields[0])
	}
}
