package compat

import "strings"

const (
	OIDBool        uint32 = 16
	OIDBytea       uint32 = 17
	OIDInt8        uint32 = 20
	OIDInt2        uint32 = 21
	OIDInt4        uint32 = 23
	OIDText        uint32 = 25
	OIDFloat4      uint32 = 700
	OIDFloat8      uint32 = 701
	OIDVarchar     uint32 = 1043
	OIDDate        uint32 = 1082
	OIDTimestamp   uint32 = 1114
	OIDTimestamptz uint32 = 1184
	OIDNumeric     uint32 = 1700
	OIDJSON        uint32 = 114
	OIDJSONB       uint32 = 3802
	OIDUUID        uint32 = 2950
)

func TypeOID(declared string) uint32 {
	t := strings.ToLower(strings.TrimSpace(declared))
	switch {
	case t == "bool" || t == "boolean":
		return OIDBool
	case strings.Contains(t, "bigint") || t == "int8":
		return OIDInt8
	case strings.Contains(t, "smallint") || t == "int2":
		return OIDInt2
	case strings.Contains(t, "int"):
		return OIDInt4
	case strings.Contains(t, "double") || strings.Contains(t, "real") || t == "float8":
		return OIDFloat8
	case t == "float4":
		return OIDFloat4
	case strings.Contains(t, "char") || strings.Contains(t, "varchar"):
		return OIDVarchar
	case strings.Contains(t, "text") || t == "":
		return OIDText
	case strings.Contains(t, "blob") || strings.Contains(t, "bytea"):
		return OIDBytea
	case t == "date":
		return OIDDate
	case strings.Contains(t, "timestamptz"):
		return OIDTimestamptz
	case strings.Contains(t, "timestamp") || strings.Contains(t, "datetime"):
		return OIDTimestamp
	case strings.Contains(t, "numeric") || strings.Contains(t, "decimal"):
		return OIDNumeric
	case t == "json":
		return OIDJSON
	case t == "jsonb":
		return OIDJSONB
	case t == "uuid":
		return OIDUUID
	default:
		return OIDText
	}
}
