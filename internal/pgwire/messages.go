package pgwire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/yashikota/minipsql/internal/backend"
	"github.com/yashikota/minipsql/internal/compat"
	"github.com/yashikota/minipsql/internal/session"
)

func writeAuthOK(w io.Writer) {
	writeMsg(w, 'R', int32Bytes(0))
}

func writeParameterStatus(w io.Writer, name, value string) {
	var b bytes.Buffer
	writeCString(&b, name)
	writeCString(&b, value)
	writeMsg(w, 'S', b.Bytes())
}

func writeBackendKeyData(w io.Writer, pid, secret int32) {
	var b bytes.Buffer
	writeInt32(&b, pid)
	writeInt32(&b, secret)
	writeMsg(w, 'K', b.Bytes())
}

func writeReady(w io.Writer, state session.TxState) {
	writeMsg(w, 'Z', []byte{byte(state)})
}

func writeRowDescription(w io.Writer, cols []backend.Column) {
	var b bytes.Buffer
	writeInt16(&b, int16(len(cols)))
	for _, c := range cols {
		writeCString(&b, c.Name)
		writeInt32(&b, 0)
		writeInt16(&b, 0)
		writeUint32(&b, c.TypeOID)
		writeInt16(&b, -1)
		writeInt32(&b, -1)
		writeInt16(&b, c.Format)
	}
	writeMsg(w, 'T', b.Bytes())
}

func writeDataRow(w io.Writer, row []backend.Value, cols []backend.Column) {
	var b bytes.Buffer
	writeInt16(&b, int16(len(row)))
	for i, v := range row {
		if v.Kind == backend.ValueNull {
			writeInt32(&b, -1)
			continue
		}
		col := backend.Column{TypeOID: compat.OIDText, Format: 0}
		if i < len(cols) {
			col = cols[i]
		}
		data := dataValue(v, col)
		writeInt32(&b, int32(len(data)))
		b.Write(data)
	}
	writeMsg(w, 'D', b.Bytes())
}

func writeCommandComplete(w io.Writer, tag string) {
	var b bytes.Buffer
	writeCString(&b, tag)
	writeMsg(w, 'C', b.Bytes())
}

func writeError(w io.Writer, pgerr compat.PGError) {
	var b bytes.Buffer
	b.WriteByte('S')
	writeCString(&b, "ERROR")
	b.WriteByte('C')
	writeCString(&b, pgerr.Code)
	b.WriteByte('M')
	writeCString(&b, pgerr.Message)
	b.WriteByte(0)
	writeMsg(w, 'E', b.Bytes())
}

func writeParseComplete(w io.Writer) {
	writeMsg(w, '1', nil)
}

func writeBindComplete(w io.Writer) {
	writeMsg(w, '2', nil)
}

func writeCloseComplete(w io.Writer) {
	writeMsg(w, '3', nil)
}

func writeNoData(w io.Writer) {
	writeMsg(w, 'n', nil)
}

func writeParameterDescription(w io.Writer, stmt *session.PreparedStatement) {
	var b bytes.Buffer
	if stmt == nil {
		writeInt16(&b, 0)
	} else {
		writeInt16(&b, int16(len(stmt.ParamTypeOIDs)))
		for _, oid := range stmt.ParamTypeOIDs {
			writeUint32(&b, oid)
		}
	}
	writeMsg(w, 't', b.Bytes())
}

func writeFlush(w io.Writer) {
	if f, ok := w.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
}

func writeMsg(w io.Writer, typ byte, body []byte) {
	var b bytes.Buffer
	b.WriteByte(typ)
	writeInt32(&b, int32(len(body)+4))
	b.Write(body)
	_, _ = w.Write(b.Bytes())
}

func textValue(v backend.Value) string {
	switch v.Kind {
	case backend.ValueBool:
		if v.Value.(bool) {
			return "t"
		}
		return "f"
	case backend.ValueInt:
		return fmt.Sprint(v.Value)
	case backend.ValueFloat:
		return strconv.FormatFloat(v.Value.(float64), 'f', -1, 64)
	case backend.ValueBytes:
		return `\x` + fmt.Sprintf("%x", v.Value)
	case backend.ValueText:
		return fmt.Sprint(v.Value)
	default:
		return ""
	}
}

func dataValue(v backend.Value, col backend.Column) []byte {
	if col.Format != 1 {
		return []byte(textValue(v))
	}
	var b [8]byte
	switch col.TypeOID {
	case compat.OIDBool:
		if v.Kind == backend.ValueBool && v.Value.(bool) {
			return []byte{1}
		}
		return []byte{0}
	case compat.OIDInt2:
		binary.BigEndian.PutUint16(b[:2], uint16(asInt64(v)))
		return b[:2]
	case compat.OIDInt4:
		binary.BigEndian.PutUint32(b[:4], uint32(asInt64(v)))
		return b[:4]
	case compat.OIDInt8:
		binary.BigEndian.PutUint64(b[:], uint64(asInt64(v)))
		return b[:]
	case compat.OIDFloat4:
		binary.BigEndian.PutUint32(b[:4], math.Float32bits(float32(asFloat64(v))))
		return b[:4]
	case compat.OIDFloat8:
		binary.BigEndian.PutUint64(b[:], math.Float64bits(asFloat64(v)))
		return b[:]
	default:
		return []byte(textValue(v))
	}
}

func asInt64(v backend.Value) int64 {
	switch v.Kind {
	case backend.ValueInt:
		switch x := v.Value.(type) {
		case int64:
			return x
		case int:
			return int64(x)
		}
	case backend.ValueText:
		n, _ := strconv.ParseInt(fmt.Sprint(v.Value), 10, 64)
		return n
	}
	return 0
}

func asFloat64(v backend.Value) float64 {
	switch v.Kind {
	case backend.ValueFloat:
		return v.Value.(float64)
	case backend.ValueInt:
		return float64(asInt64(v))
	case backend.ValueText:
		f, _ := strconv.ParseFloat(fmt.Sprint(v.Value), 64)
		return f
	}
	return 0
}

func cstring(body []byte) string {
	if i := bytes.IndexByte(body, 0); i >= 0 {
		return string(body[:i])
	}
	return string(body)
}

func readCString(r *bytes.Reader) string {
	var b bytes.Buffer
	for {
		c, err := r.ReadByte()
		if err != nil || c == 0 {
			return b.String()
		}
		b.WriteByte(c)
	}
}

func readInt16(r *bytes.Reader) int16 {
	var b [2]byte
	_, _ = io.ReadFull(r, b[:])
	return int16(binary.BigEndian.Uint16(b[:]))
}

func readInt32(r *bytes.Reader) int32 {
	var b [4]byte
	_, _ = io.ReadFull(r, b[:])
	return int32(binary.BigEndian.Uint32(b[:]))
}

func readUint32(r *bytes.Reader) uint32 {
	var b [4]byte
	_, _ = io.ReadFull(r, b[:])
	return binary.BigEndian.Uint32(b[:])
}

func writeCString(w io.Writer, s string) {
	_, _ = io.WriteString(w, s)
	_, _ = w.Write([]byte{0})
}

func writeInt16(w io.Writer, v int16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(v))
	_, _ = w.Write(b[:])
}

func writeInt32(w io.Writer, v int32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	_, _ = w.Write(b[:])
}

func writeUint32(w io.Writer, v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	_, _ = w.Write(b[:])
}

func int32Bytes(v int32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	return b[:]
}
