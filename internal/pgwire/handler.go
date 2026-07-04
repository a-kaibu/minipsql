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
	"strconv"
	"strings"

	"github.com/yashikota/minipostgresql/internal/backend"
	"github.com/yashikota/minipostgresql/internal/backend/sqlite"
	"github.com/yashikota/minipostgresql/internal/compat"
	"github.com/yashikota/minipostgresql/internal/session"
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
	defer conn.Close()
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
	defer b.Close(ctx)

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

	for {
		typ, body, err := readMessage(br)
		if err != nil {
			return
		}
		switch typ {
		case 'Q':
			h.handleSimple(ctx, conn, s, cstring(body))
		case 'P':
			h.handleParse(conn, s, body)
		case 'B':
			h.handleBind(conn, s, body)
		case 'D':
			h.handleDescribe(ctx, conn, s, body)
		case 'E':
			h.handleExecute(ctx, conn, s, body)
		case 'C':
			writeCloseComplete(conn)
		case 'H':
			writeFlush(conn)
		case 'S':
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

func (h *Handler) handleSimple(ctx context.Context, w io.Writer, s *session.Session, sql string) {
	for _, stmt := range compat.SplitStatements(sql) {
		result, tag, err := s.Execute(ctx, stmt, nil)
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

func (h *Handler) handleParse(w io.Writer, s *session.Session, body []byte) {
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
}

func (h *Handler) handleBind(w io.Writer, s *session.Session, body []byte) {
	r := bytes.NewReader(body)
	portalName := readCString(r)
	stmtName := readCString(r)
	stmt, ok := s.PreparedStatements[stmtName]
	if !ok {
		writeError(w, compat.PGError{Code: "26000", Message: "prepared statement does not exist"})
		return
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
}

func (h *Handler) handleDescribe(ctx context.Context, w io.Writer, s *session.Session, body []byte) {
	if len(body) == 0 {
		writeNoData(w)
		return
	}
	kind := body[0]
	name := cstring(body[1:])
	switch kind {
	case 'S':
		stmt := s.PreparedStatements[name]
		writeParameterDescription(w, stmt)
		if stmt == nil || !compat.IsQuery(stmt.RewrittenSQL) {
			writeNoData(w)
			return
		}
		args := make([]backend.Value, len(stmt.ParamTypeOIDs))
		for i := range args {
			args[i] = backend.Value{Kind: backend.ValueNull}
		}
		result, _, err := s.Execute(ctx, stmt.SQL, args)
		if err != nil {
			writeError(w, compat.Error(err))
			return
		}
		if result == nil {
			writeNoData(w)
			return
		}
		writeRowDescription(w, result.Columns)
	case 'P':
		p := s.Portals[name]
		if p == nil {
			writeError(w, compat.PGError{Code: "34000", Message: "portal does not exist"})
			return
		}
		if err := h.ensurePortalExecuted(ctx, s, p); err != nil {
			writeError(w, compat.Error(err))
			return
		}
		if p.Result != nil {
			applyResultFormats(p.Result, p.ResultFormats)
			writeRowDescription(w, p.Result.Columns)
		} else {
			writeNoData(w)
		}
	default:
		writeNoData(w)
	}
}

func (h *Handler) handleExecute(ctx context.Context, w io.Writer, s *session.Session, body []byte) {
	r := bytes.NewReader(body)
	name := readCString(r)
	_ = readInt32(r)
	p := s.Portals[name]
	if p == nil {
		writeError(w, compat.PGError{Code: "34000", Message: "portal does not exist"})
		return
	}
	if err := h.ensurePortalExecuted(ctx, s, p); err != nil {
		writeError(w, compat.Error(err))
		return
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

func (h *Handler) ensurePortalExecuted(ctx context.Context, s *session.Session, p *session.Portal) error {
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
