package pgwire

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/backend/sqlite"
	"github.com/yashikota/minipsql/internal/compat"
	pgcopy "github.com/yashikota/minipsql/internal/copy"
	"github.com/yashikota/minipsql/internal/session"
)

const (
	sslRequestCode = 80877103
	protocol30     = 196608
)

type Handler struct {
	InitSQL string
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ServeConn(ctx context.Context, conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()
	br := bufio.NewReader(conn)
	params, ok := h.startup(ctx, br, conn)
	if !ok {
		return
	}

	b, err := sqlite.New(ctx)
	if err != nil {
		writeError(conn, compat.Error(err))
		return
	}
	defer func() {
		_ = b.Close(ctx)
	}()

	s := session.New(randInt32(), randInt32(), params["database"], params["user"], b)
	if err := h.runInitSQL(ctx, s); err != nil {
		writeError(conn, compat.Error(err))
		return
	}
	writeAuthOK(conn)
	for k, v := range s.Params {
		writeParameterStatus(conn, k, v)
	}
	writeBackendKeyData(conn, s.ID, s.Secret)
	writeReady(conn, s.TxState)

	errorPending := false
	for {
		typ, body, err := readMessage(br)
		if err != nil {
			return
		}
		if errorPending && typ != 'S' {
			continue
		}
		switch typ {
		case 'Q':
			h.handleSimple(ctx, conn, br, s, cstring(body))
		case 'P':
			if !h.handleParse(conn, s, body) {
				errorPending = true
			}
		case 'B':
			if !h.handleBind(conn, s, body) {
				errorPending = true
			}
		case 'D':
			if !h.handleDescribe(ctx, conn, s, body) {
				errorPending = true
			}
		case 'E':
			if !h.handleExecute(ctx, conn, s, body) {
				errorPending = true
			}
		case 'C':
			writeCloseComplete(conn)
		case 'H':
			writeFlush(conn)
		case 'S':
			errorPending = false
			writeReady(conn, s.TxState)
		case 'X':
			return
		default:
			writeError(conn, compat.PGError{Code: "08P01", Message: fmt.Sprintf("unsupported message type %q", typ)})
			writeReady(conn, s.TxState)
		}
	}
}

func (h *Handler) runInitSQL(ctx context.Context, s *session.Session) error {
	if strings.TrimSpace(h.InitSQL) == "" {
		return nil
	}
	for _, stmt := range compat.SplitStatements(h.InitSQL) {
		_, _, err := s.Execute(ctx, stmt, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) startup(ctx context.Context, br *bufio.Reader, w io.Writer) (map[string]string, bool) {
	_ = ctx
	for {
		lenBytes := make([]byte, 4)
		if _, err := io.ReadFull(br, lenBytes); err != nil {
			return nil, false
		}
		n := int(binary.BigEndian.Uint32(lenBytes))
		if n < 8 {
			return nil, false
		}
		body := make([]byte, n-4)
		if _, err := io.ReadFull(br, body); err != nil {
			return nil, false
		}
		code := int(binary.BigEndian.Uint32(body[:4]))
		if code == sslRequestCode {
			_, _ = w.Write([]byte{'N'})
			continue
		}
		if code != protocol30 {
			writeError(w, compat.PGError{Code: "08P01", Message: "unsupported startup protocol"})
			return nil, false
		}
		return startupParams(body[4:]), true
	}
}

func (h *Handler) handleSimple(ctx context.Context, w io.Writer, br *bufio.Reader, s *session.Session, sql string) {
	for _, stmt := range compat.SplitStatements(sql) {
		if copyStmt, _ := pgcopy.Parse(stmt); copyStmt != nil && copyStmt.Direction == "from" {
			if copyStmt.Source == "stdin" {
				h.handleCopyIn(ctx, w, br, s, copyStmt)
			} else {
				h.handleCopyFile(ctx, w, s, copyStmt)
			}
			writeReady(w, s.TxState)
			return
		}
		result, tag, err := h.executeRecover(ctx, s, stmt, nil)
		if err != nil {
			writeError(w, compat.Error(err))
			writeReady(w, s.TxState)
			return
		}
		if result != nil {
			writeRowDescription(w, result.Columns)
			for _, row := range result.Rows {
				writeDataRow(w, row, result.Columns)
			}
			writeCommandComplete(w, "SELECT "+strconv.Itoa(len(result.Rows)))
			continue
		}
		if tag != nil {
			writeCommandComplete(w, tag.Tag)
		}
	}
	writeReady(w, s.TxState)
}

func (h *Handler) executeRecover(ctx context.Context, s *session.Session, sql string, args []backend.Value) (result *backend.Result, tag *backend.CommandTag, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	return s.Execute(ctx, sql, args)
}

func (h *Handler) handleParse(w io.Writer, s *session.Session, body []byte) bool {
	r := bytes.NewReader(body)
	name := readCString(r)
	sql := readCString(r)
	n := readInt16(r)
	oids := make([]uint32, n)
	for i := range oids {
		oids[i] = readUint32(r)
	}
	if len(oids) == 0 {
		oids = make([]uint32, compat.ParamCount(sql))
	}
	s.PreparedStatements[name] = &session.PreparedStatement{Name: name, SQL: sql, ParamTypeOIDs: oids, RewrittenSQL: compat.Rewrite(sql)}
	writeParseComplete(w)
	return true
}

func (h *Handler) handleBind(w io.Writer, s *session.Session, body []byte) bool {
	r := bytes.NewReader(body)
	portalName := readCString(r)
	stmtName := readCString(r)
	stmt, ok := s.PreparedStatements[stmtName]
	if !ok {
		writeError(w, compat.PGError{Code: "26000", Message: "prepared statement does not exist"})
		return false
	}
	formatCount := int(readInt16(r))
	formats := make([]int16, formatCount)
	for i := range formats {
		formats[i] = readInt16(r)
	}
	paramCount := int(readInt16(r))
	args := make([]backend.Value, paramCount)
	for i := range args {
		l := readInt32(r)
		if l < 0 {
			args[i] = backend.Value{Kind: backend.ValueNull}
			continue
		}
		buf := make([]byte, l)
		_, _ = io.ReadFull(r, buf)
		format := int16(0)
		if len(formats) == 1 {
			format = formats[0]
		} else if i < len(formats) {
			format = formats[i]
		}
		args[i] = decodeParam(buf, format, oidAt(stmt.ParamTypeOIDs, i))
	}
	resultFormatCount := int(readInt16(r))
	resultFormats := make([]int16, resultFormatCount)
	for i := range resultFormats {
		resultFormats[i] = readInt16(r)
	}
	s.Portals[portalName] = &session.Portal{Name: portalName, StatementName: stmtName, Args: args, ResultFormats: resultFormats}
	writeBindComplete(w)
	return true
}

func (h *Handler) handleDescribe(ctx context.Context, w io.Writer, s *session.Session, body []byte) bool {
	if len(body) == 0 {
		writeNoData(w)
		return true
	}
	kind := body[0]
	name := cstring(body[1:])
	switch kind {
	case 'S':
		stmt := s.PreparedStatements[name]
		writeParameterDescription(w, stmt)
		if stmt == nil || !compat.IsQuery(stmt.RewrittenSQL) {
			writeNoData(w)
			return true
		}
		args := make([]backend.Value, len(stmt.ParamTypeOIDs))
		for i := range args {
			args[i] = backend.Value{Kind: backend.ValueNull}
		}
		result, _, err := s.Execute(ctx, stmt.SQL, args)
		if err != nil {
			writeError(w, compat.Error(err))
			return false
		}
		if result == nil {
			writeNoData(w)
			return true
		}
		writeRowDescription(w, result.Columns)
		return true
	case 'P':
		p := s.Portals[name]
		if p == nil {
			writeError(w, compat.PGError{Code: "34000", Message: "portal does not exist"})
			return false
		}
		if err := h.ensurePortalExecuted(ctx, s, p); err != nil {
			writeError(w, compat.Error(err))
			return false
		}
		if p.Result != nil {
			applyResultFormats(p.Result, p.ResultFormats)
			writeRowDescription(w, p.Result.Columns)
		} else {
			writeNoData(w)
		}
		return true
	default:
		writeNoData(w)
		return true
	}
}

func (h *Handler) handleExecute(ctx context.Context, w io.Writer, s *session.Session, body []byte) bool {
	r := bytes.NewReader(body)
	name := readCString(r)
	_ = readInt32(r)
	p := s.Portals[name]
	if p == nil {
		writeError(w, compat.PGError{Code: "34000", Message: "portal does not exist"})
		return false
	}
	if err := h.ensurePortalExecuted(ctx, s, p); err != nil {
		writeError(w, compat.Error(err))
		return false
	}
	if p.Result != nil {
		applyResultFormats(p.Result, p.ResultFormats)
		if len(p.Result.Columns) > 0 {
			for _, row := range p.Result.Rows {
				writeDataRow(w, row, p.Result.Columns)
			}
		}
		writeCommandComplete(w, "SELECT "+strconv.Itoa(len(p.Result.Rows)))
	} else if p.CommandTag != nil {
		writeCommandComplete(w, p.CommandTag.Tag)
	}
	return true
}

func applyResultFormats(result *backend.Result, formats []int16) {
	if result == nil || len(formats) == 0 {
		return
	}
	for i := range result.Columns {
		if len(formats) == 1 {
			result.Columns[i].Format = formats[0]
			continue
		}
		if i < len(formats) {
			result.Columns[i].Format = formats[i]
		}
	}
}

func (h *Handler) ensurePortalExecuted(ctx context.Context, s *session.Session, p *session.Portal) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	if p.Result != nil || p.CommandTag != nil {
		return nil
	}
	stmt := s.PreparedStatements[p.StatementName]
	if stmt == nil {
		return errors.New("prepared statement does not exist")
	}
	result, tag, err := s.Execute(ctx, stmt.SQL, p.Args)
	p.Result = result
	p.CommandTag = tag
	return err
}

func startupParams(body []byte) map[string]string {
	params := map[string]string{}
	parts := bytes.Split(body, []byte{0})
	for i := 0; i+1 < len(parts); i += 2 {
		k := string(parts[i])
		if k == "" {
			break
		}
		params[k] = string(parts[i+1])
	}
	return params
}

func readMessage(r *bufio.Reader) (byte, []byte, error) {
	typ, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	lenBytes := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBytes); err != nil {
		return 0, nil, err
	}
	n := int(binary.BigEndian.Uint32(lenBytes))
	if n < 4 {
		return 0, nil, errors.New("bad message length")
	}
	body := make([]byte, n-4)
	_, err = io.ReadFull(r, body)
	return typ, body, err
}

func decodeParam(buf []byte, format int16, oid uint32) backend.Value {
	if format == 1 {
		switch oid {
		case compat.OIDInt2:
			if len(buf) == 2 {
				return backend.Value{Kind: backend.ValueInt, Value: int64(int16(binary.BigEndian.Uint16(buf)))}
			}
		case compat.OIDInt4:
			if len(buf) == 4 {
				return backend.Value{Kind: backend.ValueInt, Value: int64(int32(binary.BigEndian.Uint32(buf)))}
			}
		case compat.OIDInt8:
			if len(buf) == 8 {
				return backend.Value{Kind: backend.ValueInt, Value: int64(binary.BigEndian.Uint64(buf))}
			}
		}
	}
	s := string(buf)
	if oid == compat.OIDInt2 || oid == compat.OIDInt4 || oid == compat.OIDInt8 {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return backend.Value{Kind: backend.ValueInt, Value: n}
		}
	}
	if oid == compat.OIDFloat4 || oid == compat.OIDFloat8 {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return backend.Value{Kind: backend.ValueFloat, Value: f}
		}
	}
	if strings.EqualFold(s, "true") || s == "t" {
		return backend.Value{Kind: backend.ValueBool, Value: true}
	}
	if strings.EqualFold(s, "false") || s == "f" {
		return backend.Value{Kind: backend.ValueBool, Value: false}
	}
	return backend.Value{Kind: backend.ValueText, Value: s}
}

func oidAt(oids []uint32, i int) uint32 {
	if i < len(oids) && oids[i] != 0 {
		return oids[i]
	}
	return compat.OIDText
}

func randInt32() int32 {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return int32(binary.BigEndian.Uint32(b[:]))
}

func (h *Handler) handleCopyIn(ctx context.Context, w io.Writer, br *bufio.Reader, s *session.Session, stmt *pgcopy.Statement) {
	defer func() {
		if r := recover(); r != nil {
			writeError(w, compat.PGError{Code: "XX000", Message: fmt.Sprintf("internal error during COPY: %v", r)})
		}
	}()
	// resolve column count for CopyInResponse
	numCols := int16(len(stmt.Columns))
	if numCols == 0 {
		cols, err := s.Backend.TableColumns(ctx, stmt.TableName)
		if err != nil {
			writeError(w, compat.Error(err))
			return
		}
		numCols = int16(len(cols))
	}
	writeCopyInResponse(w, numCols)

	var dataBuf bytes.Buffer
	for {
		typ, body, err := readMessage(br)
		if err != nil {
			return
		}
		switch typ {
		case 'd':
			dataBuf.Write(body)
		case 'c': // CopyDone
			tag, err := s.ExecuteCopyFrom(ctx, stmt, dataBuf.Bytes())
			if err != nil {
				writeError(w, compat.Error(err))
			} else {
				writeCommandComplete(w, tag.Tag)
			}
			return
		case 'f': // CopyFail
			msg := cstring(body)
			writeError(w, compat.PGError{Code: "57014", Message: "COPY failed: " + msg})
			return
		default:
			writeError(w, compat.PGError{Code: "08P01", Message: fmt.Sprintf("unexpected message during COPY: %q", typ)})
			return
		}
	}
}

func (h *Handler) handleCopyFile(ctx context.Context, w io.Writer, s *session.Session, stmt *pgcopy.Statement) {
	data, err := readFile(stmt.Source)
	if err != nil {
		writeError(w, compat.PGError{Code: "58030", Message: fmt.Sprintf("COPY: cannot read file %q: %v", stmt.Source, err)})
		return
	}
	tag, err := s.ExecuteCopyFrom(ctx, stmt, data)
	if err != nil {
		writeError(w, compat.Error(err))
		return
	}
	writeCommandComplete(w, tag.Tag)
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
