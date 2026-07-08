package compat

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
)

var (
	truncateRE         = regexp.MustCompile(`(?i)^\s*TRUNCATE\s+(?:TABLE\s+)?([A-Za-z0-9_".,\s]+?)(?:\s+(?:CASCADE|RESTRICT))?\s*;?\s*$`)
	paramRE            = regexp.MustCompile(`\$([0-9]+)`)
	serialRE           = regexp.MustCompile(`(?i)\bserial\b`)
	boolRE             = regexp.MustCompile(`(?i)\bboolean\b|\bbool\b`)
	nowRE              = regexp.MustCompile(`(?i)\bnow\(\)`)
	quotedCharRE       = regexp.MustCompile(`::"char"`)
	castRE             = regexp.MustCompile(`::([A-Za-z_](?:[A-Za-z0-9_ ]*[A-Za-z0-9_])?(?:\([^)]*\))?(?:\[\])?)`)
	charLiteralRE      = regexp.MustCompile(`(?i)\bchar\s+('(?:''|[^'])*')`)
	pgLsnLiteralRE     = regexp.MustCompile(`(?i)\bpg_lsn\s+('(?:''|[^'])*')`)
	pointLiteralRE     = regexp.MustCompile(`(?i)\bpoint\s+('(?:''|[^'])*')`)
	typedLiteralRE     = regexp.MustCompile(`(?i)\b(int2|int4|int8|float4|float8|numeric|decimal|real|double|integer|smallint|bigint|text|varchar|bpchar|char|pg_lsn|point|bit|varbit|date|time|timestamp|timestamptz|interval|macaddr|macaddr8|uuid|regclass|regtype|oid|regproc|regoper|circle|path|line|lseg|polygon|box|cidr|inet|name|txid_snapshot|pg_snapshot|xid|xid8|tid|cid|money|json|jsonb|bytea)\s+('(?:''|[^'])*')`)
	createIndexUsingRE = regexp.MustCompile(`(?i)(CREATE\s+(?:UNIQUE\s+)?INDEX\b.+?\bON\s+\S+)\s+USING\s+\S+\s*(\()`)
	ilikeRE            = regexp.MustCompile(`(?i)\bILIKE\b`)
	returningRE        = regexp.MustCompile(`(?i)\bRETURNING\b`)
	// Rewrites "RETURNING table.*" → "RETURNING *" because SQLite rejects table-qualified wildcards
	returningTableStarRE = regexp.MustCompile(`(?i)\bRETURNING\s+(?:[A-Za-z0-9_"]+\.\*(?:\s*,\s*[A-Za-z0-9_"]+\.\*)*)`)
	onlyRE             = regexp.MustCompile(`(?i)\b(FROM|ON|UPDATE|DELETE\s+FROM|INSERT\s+INTO|ALTER\s+TABLE)\s+ONLY\s+`)
	eStringRE          = regexp.MustCompile(`(?i)\bE'`)
	trueRE             = regexp.MustCompile(`(?i)\bTRUE\b`)
	falseRE            = regexp.MustCompile(`(?i)\bFALSE\b`)
	ifNotExistsRE      = regexp.MustCompile(`(?i)\bIF\s+NOT\s+EXISTS\b`)
	ifExistsRE         = regexp.MustCompile(`(?i)\bIF\s+EXISTS\b`)
	deleteFromRE       = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+([A-Za-z0-9_\".]+)\s+([A-Za-z0-9_\"]+)\s+`)
	pgCatalogPrefixRE  = regexp.MustCompile(`(?i)\bpg_catalog\.([a-zA-Z0-9_]+)\b`)
	// Matches CREATE [UNIQUE] INDEX [IF NOT EXISTS] ON table — no index name
	anonymousIndexRE   = regexp.MustCompile(`(?i)\bCREATE\s+(UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?ON\s+([A-Za-z0-9_\".]+)\b`)
	circleColRE        = regexp.MustCompile(`(?i)\b\w+\s+circle\b`)
	asNullRE           = regexp.MustCompile(`(?i)\bAS\s+null\b`)
	tableColumnAliasRE = regexp.MustCompile(`(?i)\bFROM\s+(int2_tbl|int4_tbl|int8_tbl)\b\s+(?:AS\s+)?([A-Za-z0-9_]+)\s*\(\s*([A-Za-z0-9_, ]+)\s*\)`)
	// Matches: FROM/JOIN  table [AS] alias (col1, col2, ...)
	// The column list must contain only simple identifiers (no expressions) to avoid matching function calls.
	genericTableColAliasRE = regexp.MustCompile(`(?i)(?:\bFROM\b|\bJOIN\b)\s+([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\s+(?:AS\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*\)`)
	tableKeywordRE     = regexp.MustCompile(`(?i)(^|[^a-zA-Z0-9_.])(?:([a-zA-Z0-9_]+)\s+)?\bTABLE\s+([A-Za-z0-9_]+)\b`)
	arrayConstructorRE  = regexp.MustCompile(`(?is)\bARRAY\s*\[(.*?)\]`)
	arrayIndexRE        = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\[(\d+)\]`)
	arrayVarIndexRE     = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\[([A-Za-z_][A-Za-z0-9_.]*)\]`)
	usingLessRE         = regexp.MustCompile(`(?i)\busing\s+<`)
	usingGreaterRE      = regexp.MustCompile(`(?i)\busing\s+>`)
	// Match tablename* (PG inheritance asterisk) only when followed by whitespace, comma, ), or end of string.
	// Does NOT match tablename.* (that's a qualified wildcard, SQLite handles it natively)
	// Does NOT match g*10 (arithmetic) since the asterisk is followed by a digit or identifier.
	asteriskTableRE     = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+)\*(\s|,|\)|$)`)
	parenthesizedStartSelectRE = regexp.MustCompile(`(?i)^\s*\(\s*(SELECT\b[^)]+?)\)`)
	parenthesizedSetSelectRE   = regexp.MustCompile(`(?i)\b(UNION\s+ALL|UNION|EXCEPT|INTERSECT)\s*\(\s*(SELECT\b[^)]+?)\)`)
	extractRE          = regexp.MustCompile(`(?i)\bEXTRACT\s*\(\s*([A-Za-z_]+)\s+FROM\s+`)
	schemaPrefixRE     = regexp.MustCompile(`(?i)\b(?:public|pg_temp|pg_catalog|information_schema)\s*\.\s*([A-Za-z0-9_"]+)`)
	createViewWithOptsRE = regexp.MustCompile(`(?i)(CREATE\s+(?:OR\s+REPLACE\s+)?(?:TEMP(?:ORARY)?\s+)?VIEW\s+(?:IF\s+NOT\s+EXISTS\s+)?\S+)\s+WITH\s*\([^)]*\)`)
	createTableWithOptsRE = regexp.MustCompile(`(?i)(CREATE\s+(?:UNLOGGED\s+|TEMP(?:ORARY)?\s+)?TABLE\s+[^\s(]+(?:\s*\([^)]*\))?)\s+WITH\s*\([^)]*\)`)
	createTableWithNoDataRE = regexp.MustCompile(`(?i)\bWITH\s+NO\s+DATA\b`)
	createTableColListRE = regexp.MustCompile(`(?i)^\s*(CREATE\s+(?:UNLOGGED\s+|TEMP(?:ORARY)?\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?[A-Za-z0-9_".]+)\s*\(([^)]+)\)\s*(AS\s+)`)
	withOrdinalityRE   = regexp.MustCompile(`(?i)\bWITH\s+ORDINALITY\b`)
	bareOffsetRE       = regexp.MustCompile(`(?i)\bOFFSET\s+(\d+)(?:\s+ROWS?)?(?:\s*[;)\s]|$)`)
	offsetBeforeLimitRE = regexp.MustCompile(`(?i)\bOFFSET\s+(\S+)\s+LIMIT\s+(\S+)`)
	dropCascadeRE       = regexp.MustCompile(`(?i)^(\s*DROP\s+(?:VIEW|INDEX|SCHEMA|SEQUENCE|FUNCTION|PROCEDURE|AGGREGATE|TYPE|DOMAIN|OPERATOR|RULE|TRIGGER|CAST|COLLATION|CONVERSION|STATISTICS|LANGUAGE|POLICY|EXTENSION|ACCESS\s+METHOD|OPERATOR\s+CLASS|OPERATOR\s+FAMILY|FOREIGN\s+(?:TABLE|DATA\s+WRAPPER|SERVER))\s+(?:IF\s+EXISTS\s+)?(?:[A-Za-z0-9_".,\s]+?))(\s+(?:CASCADE|RESTRICT))(\s*;?\s*)$`)
	pgCatalogOperatorRE  = regexp.MustCompile(`(?i)\bOPERATOR\s*\(\s*(?:pg_catalog\s*\.\s*)?(~\*?|=|<>|!=|<|>|<=|>=|\|\|)\s*\)`)
	pgCatalogCollateRE   = regexp.MustCompile(`(?i)\s+COLLATE\s+(?:pg_catalog\.[A-Za-z0-9_]+|"pg_catalog"\.[A-Za-z0-9_]+|[A-Za-z0-9_".-]+\.[A-Za-z0-9_"]+)`)
	collationForRE       = regexp.MustCompile(`(?i)\bCOLLATION\s+FOR\s*\(`)
	castCollateRE        = regexp.MustCompile(`(?i)(CAST\s*\([^)]+\s+AS\s+[A-Za-z0-9_\s]+)\s+COLLATE\s+(?:"[^"]*"|[A-Za-z0-9_]+)\s*(\))`)
	nullsDistinctRE      = regexp.MustCompile(`(?i)\s+NULLS\s+(?:NOT\s+)?DISTINCT\b`)
	// USER keyword (standalone, not part of CURRENT_USER/SESSION_USER) → current_user()
	bareUserRE           = regexp.MustCompile(`(?i)\bUSER\b`)
	partitionByClauseRE = regexp.MustCompile(`(?i)\s+PARTITION\s+BY\s+(?:RANGE|LIST|HASH)\s*\(`)
	partitionOfRE       = regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+\S+\s+PARTITION\s+OF\s+`)
	attachPartitionRE   = regexp.MustCompile(`(?i)\bATTACH\s+PARTITION\b`)
	detachPartitionRE   = regexp.MustCompile(`(?i)\bDETACH\s+PARTITION\b`)
	generatedIdentityRE = regexp.MustCompile(`(?i)\s+GENERATED\s+(?:ALWAYS|BY\s+DEFAULT)\s+AS\s+IDENTITY(?:\s*\([^)]*\))?`)
	enforcedRE          = regexp.MustCompile(`(?i)\s+(?:NOT\s+)?ENFORCED\b`)
	tableoidRE          = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+\.)?tableoid\b`)
	ctidRE              = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+\.)?ctid\b`)
	xminRE              = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+\.)?xmin\b`)
	xmaxRE              = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+\.)?xmax\b`)
	cminRE              = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+\.)?cmin\b`)
	cmaxRE              = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+\.)?cmax\b`)
	onCommitRE          = regexp.MustCompile(`(?i)\s+ON\s+COMMIT\s+(?:DELETE\s+ROWS|DROP|PRESERVE\s+ROWS)`)
	// Group 1: the parent table name inside INHERITS(...)
	inheritsRE          = regexp.MustCompile(`(?i)\s+INHERITS\s*\(([^)]+)\)`)
	generatedStoredRE   = regexp.MustCompile(`(?i)\s+GENERATED\s+ALWAYS\s+AS\s+\(([^)]+)\)\s+(?:STORED|VIRTUAL)`)
	generatedByDefaultStoredRE = regexp.MustCompile(`(?i)\s+GENERATED\s+BY\s+DEFAULT\s+AS\s+\(([^)]+)\)\s+STORED`)
	dropMultiTableRE   = regexp.MustCompile(`(?i)^\s*(DROP\s+TABLE(?:\s+IF\s+EXISTS)?\s+)([A-Za-z0-9_".,\s]+?)(\s+(?:CASCADE|RESTRICT))?\s*;?\s*$`)
	indexOpClassRE     = regexp.MustCompile(`(?i)\b([A-Za-z0-9_]+)\s+([A-Za-z0-9_]+_ops\b)(?:\s*\([^)]*\))?(?:\s+ASC|\s+DESC)?`)
	nullsFirstLastRE   = regexp.MustCompile(`(?i)\bNULLS\s+(?:FIRST|LAST)\b`)
	collateClauseRE    = regexp.MustCompile(`(?i)\bCOLLATE\s+(?:"([^"]*)"|([A-Za-z0-9_.]+))`)
	forUpdateRE        = regexp.MustCompile(`(?i)\s+FOR\s+(?:UPDATE|SHARE|NO\s+KEY\s+UPDATE|KEY\s+SHARE)(?:\s+OF\s+\S+)?(?:\s+NOWAIT|\s+SKIP\s+LOCKED)?\s*$`)
	indexIncludeRE     = regexp.MustCompile(`(?i)\bINCLUDE\s*\([^)]*\)`)
	compressionRE      = regexp.MustCompile(`(?i)\bCOMPRESSION\s+\w+\b`)
	withOidsRE         = regexp.MustCompile(`(?i)\s+WITH\s+OIDS\b|\s+WITH\s*\(\s*oids(?:\s*=\s*true)?\s*\)`)
	withoutOidsRE      = regexp.MustCompile(`(?i)\s+WITHOUT\s+OIDS\b|\s+WITH\s*\(\s*oids\s*=\s*false\s*\)`)
	emptyTableRE       = regexp.MustCompile(`(?i)(CREATE\s+(?:TEMP(?:ORARY)?\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?[A-Za-z0-9_".]+)\s*\(\s*\)`)
	insertDefaultRE    = regexp.MustCompile(`(?i)\bDEFAULT\b`)
	insertSelectRE     = regexp.MustCompile(`(?i)^\s*(INSERT\s+INTO\s+[A-Za-z0-9_".]+)\s*\(\s*(SELECT\b.+)\)\s*$`)
	indexWithOptsRE    = regexp.MustCompile(`(?is)(CREATE\s+(?:UNIQUE\s+)?INDEX\b.+?)\s+WITH\s*\([^)]*\)`)
	createIndexRE      = regexp.MustCompile(`(?i)(CREATE\s+(?:UNIQUE\s+)?INDEX\s+)(IF\s+NOT\s+EXISTS\s+|)([A-Za-z0-9_"]+)`)
	createAsExecuteRE  = regexp.MustCompile(`(?i)^\s*CREATE\s+(?:TEMP(?:ORARY)?\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z0-9_".]+)\s+AS\s+EXECUTE\b`)
	anyArrayRE         = regexp.MustCompile(`(?i)=\s*ANY\s*\(\s*ARRAY\s*\[([^\]]*)\](?:::[A-Za-z0-9_\[\]]+)?\s*\)`)
	notAnyArrayRE      = regexp.MustCompile(`(?i)(?:<>|!=)\s*ANY\s*\(\s*ARRAY\s*\[([^\]]*)\](?:::[A-Za-z0-9_\[\]]+)?\s*\)`)
	notAllArrayRE      = regexp.MustCompile(`(?i)(?:<>|!=)\s*ALL\s*\(\s*ARRAY\s*\[([^\]]*)\](?:::[A-Za-z0-9_\[\]]+)?\s*\)`)
	namedArgRE         = regexp.MustCompile(`(?i)\b[A-Za-z_][A-Za-z0-9_]*\s*=>\s*`)
	forPortionOfRE     = regexp.MustCompile(`(?i)\s+FOR\s+PORTION\s+OF\s+\S+\s+FROM\s+\S+\s+TO\s+\S+`)
	bitLiteralRE       = regexp.MustCompile(`(?i)\bB'([01 \t]*)'`)
	hexLiteralSpaceRE  = regexp.MustCompile(`(?i)\bX'([0-9a-fA-F \t]*)'`)
	variadicRE         = regexp.MustCompile(`(?i)\bVARIADIC\s+`)
	// timestamp/time WITH/WITHOUT TIME ZONE — strip the WITH/WITHOUT TIME ZONE suffix
	withoutTimeZoneRE  = regexp.MustCompile(`(?i)\b(timestamp|time)(?:\s*\(\s*\d+\s*\))?\s+without\s+time\s+zone\b`)
	withTimeZoneRE     = regexp.MustCompile(`(?i)\b(timestamp|time)(?:\s*\(\s*\d+\s*\))?\s+with\s+time\s+zone\b`)
	// localtime / localtimestamp — PG-specific aliases for current_time/current_timestamp
	localtimestampPrecRE = regexp.MustCompile(`(?i)\blocaltimestamp\s*\(\s*\d+\s*\)`)
	localtimePrecRE      = regexp.MustCompile(`(?i)\blocaltime\s*\(\s*\d+\s*\)`)
	localtimestampRE     = regexp.MustCompile(`(?i)\blocaltimestamp\b`)
	localtimeRE          = regexp.MustCompile(`(?i)\blocaltime\b`)
	// current_time(n) / current_timestamp(n) — precision versions not supported by SQLite
	currentTimePrecRE      = regexp.MustCompile(`(?i)\bcurrent_time\s*\(\s*\d+\s*\)`)
	currentTimestampPrecRE = regexp.MustCompile(`(?i)\bcurrent_timestamp\s*\(\s*\d+\s*\)`)
	// ROW(...) constructor — strip the ROW keyword, keep the parens
	rowConstructorRE = regexp.MustCompile(`(?i)\bROW\s*\(`)
	// Window EXCLUDE clause — SQLite does not support EXCLUDE in window frames
	windowExcludeRE = regexp.MustCompile(`(?i)\s+EXCLUDE\s+(?:NO\s+OTHERS|CURRENT\s+ROW|GROUP|TIES)\b`)
	// DEFERRABLE / INITIALLY DEFERRED|IMMEDIATE — SQLite ignores these on constraints
	deferRE = regexp.MustCompile(`(?i)\s+DEFERRABLE(?:\s+INITIALLY\s+(?:DEFERRED|IMMEDIATE))?|\s+INITIALLY\s+(?:DEFERRED|IMMEDIATE)`)
	// WITH [CASCADED|LOCAL] CHECK OPTION — SQLite view syntax doesn't support this
	withCheckOptionRE = regexp.MustCompile(`(?i)\s+WITH\s+(?:CASCADED\s+|LOCAL\s+)?CHECK\s+OPTION\b`)
)

var idxCounter atomic.Int64

// RewritePerStatement applies rewrites that must run on individual statements
// (not on multi-statement strings). Called by session after splitting.
func RewritePerStatement(sql string) string {
	sql = rewriteAlterAddColumnDefaultExpr(sql)
	sql = rewriteCreateTableDefaultExpr(sql)
	return sql
}

func Rewrite(sql string) string {
	out := rewriteAlterTableMultiAction(sql)
	out = rewriteCreateTableColListAsValues(out)
	out = rewriteDistinctOn(out)
	out = rewriteTruncate(out)
	// Strip RECURSIVE keyword from CREATE [OR REPLACE] RECURSIVE VIEW
	out = regexp.MustCompile(`(?i)(CREATE\s+(?:OR\s+REPLACE\s+)?)RECURSIVE\s+(VIEW\s+)`).ReplaceAllString(out, "${1}${2}")
	out = rewriteCreateOrReplaceView(out)
	out = rewriteCreateMaterializedView(out)
	out = rewriteDropMaterializedView(out)
	out = rewriteCreateView(out)
	out = rewriteCreateAsExecute(out)
	out = rewriteUnloggedTable(out)
	out = rewriteSelectInto(out)
	out = rewriteIndexConcurrently(out)
	out = rewriteFetchFirst(out)
	out = rewriteLateral(out)
	out = windowExcludeRE.ReplaceAllString(out, "")
	out = rewriteTablesample(out)
	// SQLite does not support EXCEPT ALL or INTERSECT ALL; strip the ALL modifier.
	out = regexp.MustCompile(`(?i)\b(EXCEPT|INTERSECT)\s+ALL\b`).ReplaceAllString(out, "$1")
	// PostgreSQL allows SELECT FROM table with no column list (returns zero-column rows).
	// SQLite requires at least one column; rewrite to SELECT NULL FROM.
	out = regexp.MustCompile(`(?i)\bSELECT\s+(FROM\b)`).ReplaceAllString(out, "SELECT NULL $1")
	// Strip WITH RECURSIVE SEARCH/CYCLE clauses (SQL:1999 standard, not supported by SQLite)
	out = regexp.MustCompile(`(?i)\)\s*SEARCH\s+(?:DEPTH|BREADTH)\s+FIRST\s+BY\s+[^)]+?\s+SET\s+\w+`).ReplaceAllString(out, ")")
	out = regexp.MustCompile(`(?i)\)\s*CYCLE\s+[^)]+?\s+SET\s+\w+\s+(?:TO\s+[^\s]+\s+DEFAULT\s+[^\s]+\s+)?USING\s+\w+`).ReplaceAllString(out, ")")
	out = rewriteExtract(out)
	out = rewriteCollationFor(out)
	out = castCollateRE.ReplaceAllString(out, "$1$2")
	out = pgCatalogCollateRE.ReplaceAllString(out, "")
	out = nullsDistinctRE.ReplaceAllString(out, "")
	out = deferRE.ReplaceAllString(out, "")
	out = withCheckOptionRE.ReplaceAllString(out, "")
	out = rewriteStandaloneUser(out)
	out = rewriteSchemaPrefix(out)
	out = rewriteViewOptions(out)
	out = rewriteTableWithOptions(out)
	out = rewriteCreateTableWithNoData(out)
	out = rewriteCreateTableColList(out)
	out = rewriteCreateTableAsParenSelect(out)
	out = rewriteDropCascade(out)
	out = rewritePGCatalogOperator(out)
	out = rewriteGeneratedColumns(out)
	out = enforcedRE.ReplaceAllString(out, "")
	out = tableoidRE.ReplaceAllString(out, "0")
	out = ctidRE.ReplaceAllString(out, "'(0,1)'")
	// Only rewrite system columns (xmin/xmax/cmin/cmax) in non-DDL statements.
	// In DDL (CREATE/ALTER TABLE), these can be used as user column names.
	lowerOut := strings.ToLower(out)
	isDDL := strings.HasPrefix(lowerOut, "create ") || strings.HasPrefix(lowerOut, "alter ") || strings.HasPrefix(lowerOut, "drop ")
	if !isDDL {
		out = xminRE.ReplaceAllString(out, "0")
		out = xmaxRE.ReplaceAllString(out, "0")
		out = cminRE.ReplaceAllString(out, "0")
		out = cmaxRE.ReplaceAllString(out, "0")
	}
	out = onCommitRE.ReplaceAllString(out, "")
	// Rewrite "CREATE TABLE t () INHERITS (parent)" → "CREATE TABLE t (LIKE parent)"
	// so expandTableLike in session.go can copy the parent's columns via schema introspection.
	out = inheritsRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := inheritsRE.FindStringSubmatch(m)
		if sub == nil {
			return ""
		}
		parent := strings.TrimSpace(sub[1])
		return " (LIKE " + parent + ")"
	})
	// Remove empty column list that preceded the INHERITS clause: "t () (LIKE p)" → "t (LIKE p)"
	out = regexp.MustCompile(`(?i)(CREATE\s+(?:TEMP(?:ORARY)?\s+)?TABLE\s+\S+)\s*\(\s*\)\s*\(LIKE`).
		ReplaceAllString(out, "$1 (LIKE")
	out = rewritePartitionBy(out)
	out = rewriteLimitParens(out)
	out = rewriteOffsetBeforeLimit(out)
	out = rewriteBareOffset(out)
	out = rewriteDropMultiTable(out)
	out = rewriteIndexOpClasses(out)
	out = rewriteForUpdate(out)
	out = rewriteIndexInclude(out)
	out = indexWithOptsRE.ReplaceAllString(out, "$1")
	out = rewriteCreateIndex(out)
	out = notAllArrayRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := notAllArrayRE.FindStringSubmatch(m)
		return "NOT IN (" + sub[1] + ")"
	})
	out = notAnyArrayRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := notAnyArrayRE.FindStringSubmatch(m)
		return "NOT IN (" + sub[1] + ")"
	})
	out = anyArrayRE.ReplaceAllString(out, "IN ($1)")
	out = namedArgRE.ReplaceAllString(out, "")
	out = forPortionOfRE.ReplaceAllString(out, "")
	out = variadicRE.ReplaceAllString(out, "")
	out = rowConstructorRE.ReplaceAllString(out, "(")
	out = withoutTimeZoneRE.ReplaceAllString(out, "$1")
	out = withTimeZoneRE.ReplaceAllString(out, "$1")
	out = rewriteCollateClause(out)
	out = rewriteColumnCompression(out)
	out = rewriteWithOids(out)
	out = rewriteEmptyTable(out)
	out = rewriteInsertDefault(out)
	out = rewriteUpdateMultiColumn(out)
	out = rewriteUpdateDefault(out)
	out = rewriteInsertSelect(out)
	out = rewriteUnicodeLiterals(out)
	out = rewriteValuesAlias(out)
	out = tableKeywordRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := tableKeywordRE.FindStringSubmatch(m)
		prefix := sub[1]
		pred := strings.ToLower(sub[2])
		tableName := sub[3]
		if pred == "create" || pred == "drop" || pred == "alter" || pred == "truncate" || pred == "exists" || pred == "temp" || pred == "temporary" || pred == "foreign" || pred == "into" || pred == "references" || pred == "key" || pred == "like" || pred == "inherits" {
			return m
		}
		res := prefix
		if sub[2] != "" {
			idx := strings.Index(strings.ToLower(m), pred)
			res = m[:idx+len(pred)] + " SELECT * FROM " + tableName
		} else {
			res += "SELECT * FROM " + tableName
		}
		return res
	})
	out = rewriteTableColumnAlias(out)
	out = rewriteGenericTableColAlias(out)
	out = rewriteUpdateAlias(out)
	out = arrayConstructorRE.ReplaceAllString(out, "minipsql_array($1)")
	out = usingLessRE.ReplaceAllString(out, "")
	out = usingGreaterRE.ReplaceAllString(out, "DESC")
	out = asteriskTableRE.ReplaceAllString(out, "$1$2")
	out = parenthesizedStartSelectRE.ReplaceAllString(out, "SELECT * FROM ($1)")
	out = parenthesizedSetSelectRE.ReplaceAllString(out, "$1 SELECT * FROM ($2)")
	out = rewriteStringContinuation(out)
	out = rewriteDollarQuotes(out)
	out = rewriteRegexFunctions(out)
	out = rewriteSubstring(out)
	out = rewriteUnnest(out)
	out = rewriteWithOrdinality(out) // strip any remaining WITH ORDINALITY not handled by unnest
	out = rewriteToBinOctHex(out)
	out = rewriteParametersAndCasts(out)
	out = serialRE.ReplaceAllString(out, "integer")
	out = boolRE.ReplaceAllString(out, "integer")
	out = nowRE.ReplaceAllString(out, "datetime('now')")
	out = localtimestampPrecRE.ReplaceAllString(out, "datetime('now')")
	out = localtimePrecRE.ReplaceAllString(out, "time('now')")
	out = localtimestampRE.ReplaceAllString(out, "datetime('now')")
	out = localtimeRE.ReplaceAllString(out, "time('now')")
	out = currentTimestampPrecRE.ReplaceAllString(out, "current_timestamp")
	out = currentTimePrecRE.ReplaceAllString(out, "current_time")
	out = quotedCharRE.ReplaceAllString(out, "::text")
	out = rewriteBitHexLiterals(out)
	out = rewritePGCast(out)
	out = charLiteralRE.ReplaceAllString(out, "$1")
	out = pgLsnLiteralRE.ReplaceAllString(out, "$1")
	out = pointLiteralRE.ReplaceAllString(out, "$1")
	// rewriteGenerateSeries before typedLiteralRE so INTERVAL 'x' literals are still recognizable.
	out = rewriteGenerateSeries(out)
	out = typedLiteralRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := typedLiteralRE.FindStringSubmatch(m)
		tName := strings.ToLower(sub[1])
		if tName == "char" || tName == "pg_lsn" || tName == "point" {
			return sub[2]
		}
		mapped := mapPGType(tName)
		return "CAST(" + sub[2] + " AS " + mapped + ")"
	})
	out = rewritePGNumericOps(out)
	out = rewriteAnonymousIndex(out)
	out = rewriteReturningTableStar(out)
	out = createIndexUsingRE.ReplaceAllString(out, "$1 $2")
	out = ilikeRE.ReplaceAllString(out, "LIKE")
	out = rewriteRegexOps(out)
	out = rewriteSimilarTo(out)
	out = rewriteNullsOrder(out)
	out = rewriteSubqueryColumnAlias(out)
	out = rewritePosition(out)
	out = rewriteOverlay(out)
	out = rewriteTrim(out)
	out = rewriteDeleteAlias(out)
	out = rewriteDistanceOperator(out)
	out = circleColRE.ReplaceAllStringFunc(out, func(m string) string {
		parts := strings.Fields(m)
		first := strings.ToLower(parts[0])
		if first == "as" || first == "cast" || first == "type" || first == "using" {
			return m
		}
		return parts[0] + " text CHECK (is_valid_circle(" + parts[0] + "))"
	})
	out = pgCatalogPrefixRE.ReplaceAllString(out, "$1")
	out = onlyRE.ReplaceAllStringFunc(out, func(m string) string {
		upper := strings.ToUpper(m)
		if strings.HasPrefix(upper, "FROM") {
			return "FROM "
		}
		if strings.HasPrefix(upper, "UPDATE") {
			return "UPDATE "
		}
		if strings.HasPrefix(upper, "DELETE") {
			return "DELETE FROM "
		}
		if strings.HasPrefix(upper, "INSERT") {
			return "INSERT INTO "
		}
		if strings.HasPrefix(upper, "ALTER") {
			return "ALTER TABLE "
		}
		return "ON "
	})
	out = eStringRE.ReplaceAllString(out, "'")
	out = asNullRE.ReplaceAllString(out, "AS \"null\"")
	out = rewritePartitionSRFs(out)
	out = rewriteStringAgg(out)
	out = rewriteArrayAgg(out)
	out = rewriteArrayConcatStrip(out)
	out = rewriteArraySubquery(out)
	out = rewriteArrayIndex(out)
	out = rewriteGeometricOperators(out)
	out = rewriteKeywords(out)
	return out
}

var (
	// FETCH FIRST n ROWS ONLY / FETCH NEXT n ROWS ONLY (n is a simple integer)
	fetchFirstRE = regexp.MustCompile(`(?i)\bFETCH\s+(?:FIRST|NEXT)\s+(\d+)\s+ROWS?\s+(?:ONLY|WITH\s+TIES)\b`)
	// FETCH FIRST ROWS ONLY/WITH TIES (no count = 1)
	fetchFirstNoCountRE = regexp.MustCompile(`(?i)\bFETCH\s+(?:FIRST|NEXT)\s+ROWS?\s+(?:ONLY|WITH\s+TIES)\b`)
	// FETCH FIRST (expr) ROWS ONLY/WITH TIES — parenthesized expression
	fetchFirstExprRE = regexp.MustCompile(`(?i)\bFETCH\s+(?:FIRST|NEXT)\s+(\([^)]+\))\s+ROWS?\s+(?:ONLY|WITH\s+TIES)\b`)
	// FETCH FIRST NULL ROWS ONLY/WITH TIES — NULL count means no limit
	fetchFirstNullRE = regexp.MustCompile(`(?i)\bFETCH\s+(?:FIRST|NEXT)\s+NULL\s+ROWS?\s+(?:ONLY|WITH\s+TIES)\b`)
	// OFFSET n ROWS
	offsetRowsRE = regexp.MustCompile(`(?i)\bOFFSET\s+(\d+)\s+ROWS\b`)
	// LATERAL keyword before subqueries/functions
	lateralRE = regexp.MustCompile(`(?i)\bLATERAL\s+`)
	// TABLESAMPLE clause — matches method name only; arg stripping is done by rewriteTablesample()
	tablesampleStartRE = regexp.MustCompile(`(?i)\bTABLESAMPLE\s+\w+\s*\(`)

	createOrReplaceViewRE   = regexp.MustCompile(`(?i)^\s*CREATE\s+OR\s+REPLACE\s+(TEMP(?:ORARY)?\s+)?VIEW\s+`)
	createViewRE            = regexp.MustCompile(`(?i)(^\s*CREATE\s+(?:TEMP(?:ORARY)?\s+)?VIEW\s+)(IF\s+NOT\s+EXISTS\s+|)([A-Za-z0-9_".]+)`)
	createMaterializedViewRE = regexp.MustCompile(`(?i)^\s*CREATE\s+MATERIALIZED\s+VIEW\s+`)
	createUnloggedTableRE = regexp.MustCompile(`(?i)\bCREATE\s+UNLOGGED\s+TABLE\b`)
	selectIntoRE          = regexp.MustCompile(`(?i)^\s*SELECT\s+(.+?)\s+INTO\s+(?:TEMP(?:ORARY)?\s+)?(?:TABLE\s+)?([A-Za-z0-9_".]+)\s+FROM\s+`)
	indexConcurrentlyRE   = regexp.MustCompile(`(?i)\b(CREATE\s+(?:UNIQUE\s+)?INDEX|DROP\s+INDEX)\s+CONCURRENTLY\s+`)
	// strip "expr || ARRAY(SELECT ...)" — e.g. c.reloptions || array(select ...) → c.reloptions
	pgArrayConcatRE        = regexp.MustCompile(`(?i)\s*\|\|\s*array\s*\(`)
)

// rewriteCreateOrReplaceView converts CREATE OR REPLACE VIEW to DROP VIEW IF EXISTS + CREATE VIEW.
func rewriteCreateOrReplaceView(sql string) string {
	m := createOrReplaceViewRE.FindStringIndex(sql)
	if m == nil {
		return sql
	}
	// Extract view name
	rest := sql[m[1]:]
	nameEnd := 0
	for nameEnd < len(rest) && rest[nameEnd] != ' ' && rest[nameEnd] != '\t' && rest[nameEnd] != '\n' && rest[nameEnd] != '(' {
		nameEnd++
	}
	viewName := rest[:nameEnd]
	tempClause := ""
	if sub := createOrReplaceViewRE.FindStringSubmatch(sql); sub != nil && sub[1] != "" {
		tempClause = "TEMP "
	}
	return "DROP VIEW IF EXISTS " + viewName + "; CREATE " + tempClause + "VIEW " + rest
}

var dropMaterializedViewRE = regexp.MustCompile(`(?i)^\s*DROP\s+MATERIALIZED\s+VIEW\s+(?:IF\s+EXISTS\s+)?`)
var dropMaterializedViewNameRE = regexp.MustCompile(`(?i)^\s*DROP\s+MATERIALIZED\s+VIEW\s+(IF\s+EXISTS\s+)?([A-Za-z0-9_".]+)`)

func rewriteDropMaterializedView(sql string) string {
	sub := dropMaterializedViewNameRE.FindStringSubmatch(sql)
	if sub == nil {
		return sql
	}
	viewName := sub[2]
	ifExists := ""
	if strings.TrimSpace(strings.ToUpper(sub[1])) == "IF EXISTS" {
		ifExists = "IF EXISTS "
	} else {
		ifExists = "IF EXISTS "
	}
	return "DROP TABLE " + ifExists + viewName
}

// rewriteCreateMaterializedView rewrites CREATE MATERIALIZED VIEW to CREATE TABLE AS SELECT
// so that subsequent CREATE INDEX and SELECT operations work on real tables.
func rewriteCreateMaterializedView(sql string) string {
	m := createMaterializedViewRE.FindStringIndex(sql)
	if m == nil {
		return sql
	}
	rest := sql[m[1]:]
	rest = strings.TrimSpace(rest)
	// Extract view name
	nameEnd := 0
	for nameEnd < len(rest) && rest[nameEnd] != ' ' && rest[nameEnd] != '\t' && rest[nameEnd] != '\n' && rest[nameEnd] != '(' {
		nameEnd++
	}
	viewName := rest[:nameEnd]
	remainder := strings.TrimSpace(rest[nameEnd:])
	// Strip WITH (options) before AS
	withRE := regexp.MustCompile(`(?i)^\s*WITH\s*\([^)]*\)\s*`)
	remainder = withRE.ReplaceAllString(remainder, "")
	return "DROP TABLE IF EXISTS " + viewName + "; CREATE TABLE " + viewName + " " + remainder
}

// rewriteCreateView adds IF NOT EXISTS to CREATE VIEW to avoid duplicate errors.
func rewriteCreateView(sql string) string {
	return createViewRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := createViewRE.FindStringSubmatch(m)
		if sub[2] != "" {
			return m // already has IF NOT EXISTS
		}
		return sub[1] + "IF NOT EXISTS " + sub[3]
	})
}

// rewriteCreateAsExecute rewrites CREATE TABLE ... AS EXECUTE ... to CREATE TABLE ... (_dummy integer).
func rewriteCreateAsExecute(sql string) string {
	m := createAsExecuteRE.FindStringSubmatchIndex(sql)
	if m == nil {
		return sql
	}
	tableName := sql[m[2]:m[3]]
	return "CREATE TABLE IF NOT EXISTS " + tableName + " (_dummy integer)"
}

// rewriteUnloggedTable strips UNLOGGED keyword from CREATE TABLE.
func rewriteUnloggedTable(sql string) string {
	return createUnloggedTableRE.ReplaceAllString(sql, "CREATE TABLE")
}

// rewriteSelectInto rewrites SELECT ... INTO tablename FROM ... to CREATE TABLE tablename AS SELECT ... FROM ...
func rewriteSelectInto(sql string) string {
	m := selectIntoRE.FindStringSubmatchIndex(sql)
	if m == nil {
		return sql
	}
	// m[2],m[3] = select list, m[4],m[5] = table name
	selectList := sql[m[2]:m[3]]
	tableName := sql[m[4]:m[5]]
	fromPos := m[1]
	return "CREATE TABLE " + tableName + " AS SELECT " + selectList + " FROM " + sql[fromPos:]
}

// rewriteBitHexLiterals converts PG bit-string literals B'...' to integer values
// and fixes X'...' hex literals by removing embedded spaces.
// SQLite does not support B'...' syntax; we convert to the decimal integer equivalent.
// SQLite supports X'...' but not with spaces inside.
func rewriteBitHexLiterals(sql string) string {
	out := bitLiteralRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := bitLiteralRE.FindStringSubmatch(m)
		bits := strings.ReplaceAll(strings.ReplaceAll(sub[1], " ", ""), "\t", "")
		if bits == "" {
			return "0"
		}
		val := int64(0)
		for _, c := range bits {
			if c == '0' || c == '1' {
				val = val*2 + int64(c-'0')
			}
		}
		return strconv.FormatInt(val, 10)
	})
	// Remove spaces from X'...' hex literals so SQLite can parse them.
	out = hexLiteralSpaceRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := hexLiteralSpaceRE.FindStringSubmatch(m)
		hex := strings.ReplaceAll(strings.ReplaceAll(sub[1], " ", ""), "\t", "")
		return "X'" + hex + "'"
	})
	return out
}

// rewriteFetchFirst rewrites SQL:2008 FETCH FIRST/NEXT syntax to LIMIT/OFFSET.
func rewriteFetchFirst(sql string) string {
	out := fetchFirstNullRE.ReplaceAllString(sql, "LIMIT -1")
	out = fetchFirstExprRE.ReplaceAllString(out, "LIMIT $1")
	out = fetchFirstNoCountRE.ReplaceAllString(out, "LIMIT 1")
	out = fetchFirstRE.ReplaceAllString(out, "LIMIT $1")
	out = offsetRowsRE.ReplaceAllString(out, "OFFSET $1")
	return out
}

// rewriteLateral strips the LATERAL keyword (SQLite evaluates correlated subqueries natively).
func rewriteLateral(sql string) string {
	return lateralRE.ReplaceAllString(sql, "")
}

// rewriteTablesample strips TABLESAMPLE clauses (not supported in SQLite).
// Handles nested parentheses, e.g. TABLESAMPLE bernoulli (('1'::text < '0'::text)::int).
func rewriteTablesample(sql string) string {
	for {
		loc := tablesampleStartRE.FindStringIndex(sql)
		if loc == nil {
			break
		}
		// loc[1]-1 is the position of the opening '(' already matched
		start := loc[0]
		openParen := loc[1] - 1
		depth := 1
		i := openParen + 1
		for i < len(sql) && depth > 0 {
			switch sql[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			i++
		}
		// i now points just after the closing ')' of TABLESAMPLE args
		// strip optional REPEATABLE (...) too
		repeatableRE := regexp.MustCompile(`(?i)^\s*REPEATABLE\s*\(`)
		rest := sql[i:]
		if m := repeatableRE.FindStringIndex(rest); m != nil {
			openR := m[1] - 1
			depth2 := 1
			j := openR + 1
			for j < len(rest) && depth2 > 0 {
				switch rest[j] {
				case '(':
					depth2++
				case ')':
					depth2--
				}
				j++
			}
			i += j
		}
		sql = sql[:start] + sql[i:]
	}
	return sql
}

// rewriteIndexConcurrently strips CONCURRENTLY from CREATE/DROP INDEX.
func rewriteIndexConcurrently(sql string) string {
	return indexConcurrentlyRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := indexConcurrentlyRE.FindStringSubmatch(m)
		return sub[1] + " "
	})
}

func rewriteTruncate(sql string) string {
	m := truncateRE.FindStringSubmatch(sql)
	if m == nil {
		return sql
	}
	tableList := strings.TrimSpace(m[1])
	tables := strings.Split(tableList, ",")
	var deletes []string
	for _, t := range tables {
		t = strings.TrimSpace(t)
		if t != "" {
			deletes = append(deletes, "DELETE FROM "+t)
		}
	}
	if len(deletes) == 0 {
		return sql
	}
	return strings.Join(deletes, "; ")
}

// rewriteReturningTableStar rewrites "RETURNING t.*" → "RETURNING *" to avoid SQLite's
// "RETURNING may not use TABLE.* wildcards" error.
func rewriteReturningTableStar(sql string) string {
	if !strings.Contains(strings.ToLower(sql), "returning") {
		return sql
	}
	return returningTableStarRE.ReplaceAllString(sql, "RETURNING *")
}

func rewriteAnonymousIndex(sql string) string {
	return anonymousIndexRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := anonymousIndexRE.FindStringSubmatch(m)
		uniqueVal := idxCounter.Add(1)
		uniqueName := fmt.Sprintf("idx_anon_%d", uniqueVal)

		uniqueStr := "CREATE "
		if strings.TrimSpace(sub[1]) != "" {
			uniqueStr += "UNIQUE "
		}
		uniqueStr += "INDEX " + uniqueName + " ON " + sub[2]
		return uniqueStr
	})
}

func rewriteDistanceOperator(sql string) string {
	idx := strings.Index(sql, "<->")
	if idx == -1 {
		return sql
	}

	// Parse left operand
	leftStart := idx
	for leftStart > 0 && (sql[leftStart-1] == ' ' || sql[leftStart-1] == '\t') {
		leftStart--
	}
	leftEnd := leftStart
	if leftStart > 0 && sql[leftStart-1] == '\'' {
		leftStart--
		for leftStart > 0 && sql[leftStart-1] != '\'' {
			leftStart--
		}
		if leftStart > 0 {
			leftStart--
		}
	} else if leftStart > 0 && sql[leftStart-1] == ')' {
		depth := 1
		leftStart--
		for leftStart > 0 && depth > 0 {
			leftStart--
			if sql[leftStart] == ')' {
				depth++
			} else if sql[leftStart] == '(' {
				depth--
			}
		}
		for leftStart > 0 && isIdentByte(sql[leftStart-1]) {
			leftStart--
		}
	} else {
		for leftStart > 0 && (isIdentByte(sql[leftStart-1]) || sql[leftStart-1] == '.') {
			leftStart--
		}
	}

	leftExpr := sql[leftStart:leftEnd]

	// Parse right operand
	rightStart := idx + 3
	for rightStart < len(sql) && (sql[rightStart] == ' ' || sql[rightStart] == '\t') {
		rightStart++
	}
	rightEnd := rightStart
	if rightStart < len(sql) && sql[rightStart] == '\'' {
		rightEnd++
		for rightEnd < len(sql) && sql[rightEnd] != '\'' {
			rightEnd++
		}
		if rightEnd < len(sql) {
			rightEnd++
		}
	} else if rightStart < len(sql) && isIdentByte(sql[rightStart]) {
		for rightEnd < len(sql) && (isIdentByte(sql[rightEnd]) || sql[rightEnd] == '.') {
			rightEnd++
		}
		if rightEnd < len(sql) && sql[rightEnd] == '(' {
			depth := 1
			rightEnd++
			for rightEnd < len(sql) && depth > 0 {
				if sql[rightEnd] == '(' {
					depth++
				} else if sql[rightEnd] == ')' {
					depth--
				}
				rightEnd++
			}
		}
	} else if rightStart < len(sql) && sql[rightStart] == '(' {
		depth := 1
		rightEnd++
		for rightEnd < len(sql) && depth > 0 {
			if sql[rightEnd] == '(' {
				depth++
			} else if sql[rightEnd] == ')' {
				depth--
			}
			rightEnd++
		}
	}

	rightExpr := sql[rightStart:rightEnd]

	rewritten := sql[:leftStart] + "dist_op(" + leftExpr + ", " + rightExpr + ")" + sql[rightEnd:]
	return rewriteDistanceOperator(rewritten)
}

func rewriteParametersAndCasts(sql string) string {
	var sb strings.Builder
	inQuote := false
	inDoubleQuote := false
	n := len(sql)
	for i := 0; i < n; i++ {
		ch := sql[i]
		if inQuote {
			sb.WriteByte(ch)
			if ch == '\'' {
				if i+1 < n && sql[i+1] == '\'' {
					sb.WriteByte('\'')
					i++
				} else {
					inQuote = false
				}
			}
			continue
		}
		if inDoubleQuote {
			sb.WriteByte(ch)
			if ch == '"' {
				if i+1 < n && sql[i+1] == '"' {
					sb.WriteByte('"')
					i++
				} else {
					inDoubleQuote = false
				}
			}
			continue
		}

		if ch == '\'' {
			inQuote = true
			sb.WriteByte(ch)
			continue
		}
		if ch == '"' {
			inDoubleQuote = true
			sb.WriteByte(ch)
			continue
		}

		if ch == '$' && i+1 < n && sql[i+1] >= '0' && sql[i+1] <= '9' {
			j := i + 1
			for j < n && sql[j] >= '0' && sql[j] <= '9' {
				j++
			}
			if j+1 < n && sql[j] == ':' && sql[j+1] == ':' {
				k := j + 2
				for k < n && (isIdentByte(sql[k]) || sql[k] == ' ') {
					k++
				}
				typeName := strings.TrimSpace(sql[j+2 : k])
				sb.WriteString("cast(? as ")
				sb.WriteString(mapPGType(typeName))
				sb.WriteString(")")
				i = k - 1
				continue
			} else {
				sb.WriteByte('?')
				i = j - 1
				continue
			}
		}

		sb.WriteByte(ch)
	}
	return sb.String()
}

func rewritePGCast(sql string) string {
	for {
		loc := castRE.FindStringIndex(sql)
		if loc == nil {
			break
		}
		typeName := strings.TrimSpace(castRE.FindStringSubmatch(sql[loc[0]:])[1])
		typeName, trailing := splitCastTypeTrailingSQL(typeName)
		typeName, aliasTrailing := splitCastTypeAlias(typeName)
		trailing = aliasTrailing + trailing
		typeName = strings.TrimSuffix(typeName, "[]")
		typeName = strings.TrimSpace(typeName)
		if idx := strings.Index(typeName, "("); idx >= 0 {
			typeName = typeName[:idx]
		}
		origTypeName := strings.ToLower(strings.TrimSpace(typeName))
		typeName = mapPGType(typeName)
		beforeCast := strings.TrimRightFunc(sql[:loc[0]], unicode.IsSpace)
		expr := extractCastExpr(beforeCast)
		prefix := beforeCast[:len(beforeCast)-len(expr)]
		suffix := trailing + sql[loc[1]:]
		if expr == "" {
			sql = prefix + suffix
		} else if origTypeName == "regclass" {
			// Rewrite 'tablename'::regclass to look up oid in pg_class
			sql = prefix + "(SELECT oid FROM pg_class WHERE relname = " + expr + ")" + suffix
		} else {
			sql = prefix + "CAST(" + expr + " AS " + typeName + ")" + suffix
		}
	}
	return sql
}

func splitCastTypeTrailingSQL(typeName string) (string, string) {
	lower := strings.ToLower(typeName)
	minIdx := -1
	for _, keyword := range []string{" placing ", " for ", " leading ", " trailing ", " both ", " from ", " where ", " order ", " group ", " limit ", " union ", " intersect ", " except ", " and ", " or ", " not ", " is ", " as ", " collate ", " like ", " ilike ", " similar ", " between ", " preceding", " following", " unbounded", " current"} {
		if idx := strings.Index(lower, keyword); idx >= 0 {
			if minIdx == -1 || idx < minIdx {
				minIdx = idx
			}
		}
	}
	if minIdx >= 0 {
		return strings.TrimSpace(typeName[:minIdx]), typeName[minIdx:]
	}
	if idx := strings.Index(lower, " in "); idx >= 0 {
		return strings.TrimSpace(typeName[:idx]), typeName[idx:]
	}
	if strings.HasSuffix(lower, " in") {
		idx := len(lower) - 3
		return strings.TrimSpace(typeName[:idx]), typeName[idx:]
	}
	return typeName, ""
}

func splitCastTypeAlias(typeName string) (string, string) {
	fields := strings.Fields(typeName)
	if len(fields) != 2 {
		return typeName, ""
	}
	lower := strings.ToLower(typeName)
	switch lower {
	case "double precision", "character varying":
		return typeName, ""
	}
	if strings.Contains(fields[1], "(") || strings.HasSuffix(fields[0], "[]") {
		return typeName, ""
	}
	return fields[0], " " + fields[1]
}

func mapPGType(t string) string {
	switch strings.ToLower(t) {
	case "integer", "int", "int4", "int2", "smallint", "bigint", "int8":
		return "integer"
	case "real", "float4", "float8", "double precision", "double":
		return "real"
	case "text", "varchar", "character varying", "character", "char", "bpchar", "name":
		return "text"
	case "boolean", "bool":
		return "integer"
	case "numeric", "decimal":
		return "numeric"
	case "bytea":
		return "blob"
	case "regclass", "regtype", "oid", "regproc", "regoper", "circle":
		return "text"
	case "oid8":
		return "integer"
	default:
		return "text"
	}
}

func extractCastExpr(sql string) string {
	if len(sql) == 0 {
		return ""
	}
	i := len(sql) - 1
	// Handle array subscript: expr[n] or expr[n:m] — scan back past balanced ]
	if sql[i] == ']' {
		depth := 1
		i--
		for i >= 0 && depth > 0 {
			if sql[i] == ']' {
				depth++
			} else if sql[i] == '[' {
				depth--
			}
			i--
		}
		i++
		// Now scan back the identifier/expr before '['
		for i > 0 {
			b := sql[i-1]
			if isIdentByte(b) || b == '.' {
				i--
				continue
			}
			break
		}
		return sql[i:]
	}
	if sql[i] == '\'' {
		i--
		for i >= 0 {
			if sql[i] == '\'' {
				if i > 0 && sql[i-1] == '\'' {
					i -= 2
					continue
				}
				break
			}
			i--
		}
		if i > 0 {
			prev := sql[i-1]
			switch prev {
			case 'E', 'e', 'B', 'b', 'X', 'x', 'N', 'n':
				i--
			}
		}
		// U& prefix
		if i >= 2 && (sql[i] == '&') && (sql[i-1] == 'U' || sql[i-1] == 'u') {
			i -= 2
		}
		return sql[i:]
	}
	if sql[i] == ')' {
		depth := 1
		i--
		for i >= 0 && depth > 0 {
			if sql[i] == '\'' {
				// scan backwards through string literal
				i--
				for i >= 0 {
					if sql[i] == '\'' {
						if i > 0 && sql[i-1] == '\'' {
							i -= 2
							continue
						}
						break
					}
					i--
				}
				i--
				continue
			}
			if sql[i] == ')' {
				depth++
			} else if sql[i] == '(' {
				depth--
			}
			i--
		}
		i++
		for i > 0 && isIdentByte(sql[i-1]) {
			i--
		}
		return sql[i:]
	}
	for i > 0 {
		b := sql[i-1]
		if isIdentByte(b) || b == '.' {
			i--
			continue
		}
		if (b == '-' || b == '+') && i > 1 && (sql[i-2] == 'e' || sql[i-2] == 'E') {
			i--
			continue
		}
		break
	}
	return sql[i:]
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func HasReturning(sql string) bool {
	return returningRE.MatchString(sql)
}

func SplitStatements(sql string) []string {
	var stmts []string
	var b strings.Builder
	inSingle := false
	inLineComment := false
	blockDepth := 0
	parenDepth := 0
	dollarTag := ""
	for i := 0; i < len(sql); i++ {
		if inLineComment {
			if sql[i] == '\n' {
				inLineComment = false
				b.WriteByte('\n')
			}
			continue
		}

		if blockDepth > 0 {
			if i+1 < len(sql) && sql[i] == '/' && sql[i+1] == '*' {
				blockDepth++
				i++
				continue
			}
			if i+1 < len(sql) && sql[i] == '*' && sql[i+1] == '/' {
				blockDepth--
				i++
				if blockDepth == 0 {
					b.WriteByte(' ')
				}
			}
			continue
		}

		if dollarTag != "" {
			if strings.HasPrefix(sql[i:], dollarTag) {
				b.WriteString(dollarTag)
				i += len(dollarTag) - 1
				dollarTag = ""
				continue
			}
			b.WriteByte(sql[i])
			continue
		}

		if inSingle {
			b.WriteByte(sql[i])
			if sql[i] == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					b.WriteByte(sql[i+1])
					i++
					continue
				}
				inSingle = false
			}
			continue
		}

		if i+1 < len(sql) && sql[i] == '-' && sql[i+1] == '-' {
			inLineComment = true
			i++
			continue
		}
		if i+1 < len(sql) && sql[i] == '/' && sql[i+1] == '*' {
			blockDepth = 1
			i++
			continue
		}
		if sql[i] == '\'' {
			inSingle = true
			b.WriteByte(sql[i])
			continue
		}
		if tag := dollarQuoteTagAt(sql, i); tag != "" {
			dollarTag = tag
			b.WriteString(tag)
			i += len(tag) - 1
			continue
		}

		if sql[i] == '(' {
			parenDepth++
		} else if sql[i] == ')' {
			parenDepth--
		}

		if sql[i] == ';' && parenDepth == 0 {
			if s := strings.TrimSpace(b.String()); s != "" {
				stmts = append(stmts, s)
			}
			b.Reset()
			continue
		}
		b.WriteByte(sql[i])
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

func dollarQuoteTagAt(sql string, start int) string {
	if start >= len(sql) || sql[start] != '$' {
		return ""
	}
	end := start + 1
	for end < len(sql) {
		c := sql[end]
		if c == '$' {
			return sql[start : end+1]
		}
		if !(isIdentByte(c)) {
			return ""
		}
		end++
	}
	return ""
}

func rewritePosition(sql string) string {
	result := sql
	startIdx := 0
	for {
		if startIdx >= len(result) {
			break
		}
		idx := indexOutsideQuotes(strings.ToUpper(result[startIdx:]), "POSITION")
		if idx < 0 {
			break
		}
		idx += startIdx

		// Check if it's part of an identifier
		isIdent := false
		if idx > 0 && isIdentByte(result[idx-1]) {
			isIdent = true
		}
		if idx+len("POSITION") < len(result) && isIdentByte(result[idx+len("POSITION")]) {
			isIdent = true
		}

		if isIdent {
			startIdx = idx + len("POSITION")
			continue
		}

		open := idx + len("POSITION")
		for open < len(result) && unicode.IsSpace(rune(result[open])) {
			open++
		}
		if open >= len(result) || result[open] != '(' {
			startIdx = idx + len("POSITION")
			continue
		}
		close := findMatchingParen(result, open)
		if close < 0 {
			startIdx = open + 1
			continue
		}
		body := result[open+1 : close]
		inStart, inEnd := topLevelInBounds(body)
		if inStart < 0 {
			rewrittenBody := rewritePosition(body)
			if rewrittenBody != body {
				result = result[:open+1] + rewrittenBody + result[close:]
				close = open + 1 + len(rewrittenBody)
			}
			startIdx = close + 1
			continue
		}
		needle := strings.TrimSpace(body[:inStart])
		haystack := strings.TrimSpace(body[inEnd:])
		needle = rewritePosition(needle)
		haystack = rewritePosition(haystack)

		replacement := "instr(" + haystack + ", " + needle + ")"
		result = result[:idx] + replacement + result[close+1:]
		startIdx = idx + len(replacement)
	}
	return result
}

func rewriteDeleteAlias(sql string) string {
	result := sql
	startIdx := 0
	for {
		if startIdx >= len(result) {
			break
		}
		loc := deleteFromRE.FindStringIndex(result[startIdx:])
		if loc == nil {
			break
		}
		matchStart := loc[0] + startIdx
		matchEnd := loc[1] + startIdx
		match := result[matchStart:matchEnd]

		sub := deleteFromRE.FindStringSubmatch(match)
		if len(sub) < 3 {
			startIdx = matchEnd
			continue
		}
		table := sub[1]
		alias := sub[2]
		lowerAlias := strings.ToLower(alias)
		if lowerAlias == "as" || lowerAlias == "where" || lowerAlias == "using" || lowerAlias == "returning" {
			startIdx = matchEnd
			continue
		}

		idxTable := strings.Index(strings.ToLower(match), strings.ToLower(table))
		if idxTable < 0 {
			startIdx = matchEnd
			continue
		}
		idxAlias := strings.Index(strings.ToLower(match[idxTable+len(table):]), strings.ToLower(alias))
		if idxAlias < 0 {
			startIdx = matchEnd
			continue
		}
		idxAlias += idxTable + len(table)

		rewrittenMatch := match[:idxAlias] + "AS " + match[idxAlias:]

		stmtEnd := strings.Index(result[matchEnd:], ";")
		var rest string
		var suffix string
		if stmtEnd < 0 {
			rest = result[matchEnd:]
			suffix = ""
		} else {
			rest = result[matchEnd : matchEnd+stmtEnd]
			suffix = result[matchEnd+stmtEnd:]
		}

		escapedTable := regexp.QuoteMeta(table)
		tableRefRE, err := regexp.Compile(`(?i)\b` + escapedTable + `\.`)
		if err == nil {
			rest = tableRefRE.ReplaceAllString(rest, alias+".")
		}

		result = result[:matchStart] + rewrittenMatch + rest + suffix
		startIdx = matchStart + len(rewrittenMatch) + len(rest)
	}
	return result
}

// rewriteCollationFor replaces COLLATION FOR (...) with NULL, handling nested parens.
func rewriteCollationFor(sql string) string {
	for {
		loc := collationForRE.FindStringIndex(sql)
		if loc == nil {
			break
		}
		// loc[1]-1 is the position of '(' (the regex ends with \()
		openParen := loc[1] - 1
		closeParen := findMatchingParen(sql, openParen)
		if closeParen < 0 {
			break
		}
		sql = sql[:loc[0]] + "NULL" + sql[closeParen+1:]
	}
	return sql
}

func findMatchingParen(sql string, open int) int {
	depth := 1
	inSingle := false
	for i := open + 1; i < len(sql); i++ {
		if inSingle {
			if sql[i] == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		switch sql[i] {
		case '\'':
			inSingle = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func topLevelInBounds(s string) (int, int) {
	depth := 0
	inSingle := false
	for i := 0; i < len(s); i++ {
		if inSingle {
			if s[i] == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		switch s[i] {
		case '\'':
			inSingle = true
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && (s[i] == 'I' || s[i] == 'i') && i+1 < len(s) && (s[i+1] == 'N' || s[i+1] == 'n') {
				beforeOK := i == 0 || unicode.IsSpace(rune(s[i-1]))
				after := i + len("in")
				afterOK := after >= len(s) || unicode.IsSpace(rune(s[after]))
				if beforeOK && afterOK {
					return i, after
				}
			}
		}
	}
	return -1, -1
}

func IsQuery(sql string) bool {
	first := firstWord(sql)
	switch strings.ToLower(first) {
	case "select", "with", "pragma":
		return true
	default:
		return false
	}
}

func TxCommand(sql string) string {
	q := strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	lower := strings.ToLower(q)
	switch {
	case lower == "begin" || lower == "begin transaction" || lower == "start transaction" ||
		strings.HasPrefix(lower, "begin isolation level") || strings.HasPrefix(lower, "begin transaction isolation level") ||
		strings.HasPrefix(lower, "start transaction isolation level") ||
		strings.HasPrefix(lower, "start transaction read") ||
		strings.HasPrefix(lower, "begin transaction read") ||
		strings.HasPrefix(lower, "begin transaction deferrable") ||
		strings.HasPrefix(lower, "begin read"):
		return "begin"
	case lower == "commit" || lower == "end" || lower == "commit transaction" || lower == "commit work":
		return "commit"
	case lower == "rollback" || lower == "abort" || lower == "rollback transaction" || lower == "rollback work":
		return "rollback"
	default:
		return ""
	}
}

func ParamCount(sql string) int {
	max := 0
	for _, match := range paramRE.FindAllStringSubmatch(sql, -1) {
		if len(match) < 2 {
			continue
		}
		n, err := strconv.Atoi(match[1])
		if err == nil && n > max {
			max = n
		}
	}
	return max
}

func firstWord(sql string) string {
	s := strings.TrimSpace(sql)
	for i, r := range s {
		if unicode.IsSpace(r) || r == ';' {
			return s[:i]
		}
	}
	return s
}

func rewriteDollarQuotes(sql string) string {
	var b strings.Builder
	i := 0
	for i < len(sql) {
		if sql[i] == '$' {
			j := i + 1
			for j < len(sql) && isAlphaNumericOrEmpty(sql[j]) {
				j++
			}
			if j < len(sql) && sql[j] == '$' {
				tag := sql[i : j+1]
				closeIdx := strings.Index(sql[j+1:], tag)
				if closeIdx != -1 {
					closeStart := (j + 1) + closeIdx
					content := sql[j+1 : closeStart]
					escaped := strings.ReplaceAll(content, "'", "''")
					b.WriteString("'" + escaped + "'")
					i = closeStart + len(tag)
					continue
				}
			}
		}
		b.WriteByte(sql[i])
		i++
	}
	return b.String()
}

func isAlphaNumericOrEmpty(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-'
}

func rewriteRegexFunctions(sql string) string {
	lower := strings.ToLower(sql)
	
	for {
		idx := strings.Index(lower, "regexp_split_to_table")
		if idx == -1 {
			break
		}
		opIdx := idx + len("regexp_split_to_table")
		for opIdx < len(sql) && (sql[opIdx] == ' ' || sql[opIdx] == '\t' || sql[opIdx] == '\n' || sql[opIdx] == '\r') {
			opIdx++
		}
		if opIdx >= len(sql) || sql[opIdx] != '(' {
			lower = lower[:idx] + "REGEXP_SPLIT_TO_TABLE" + lower[opIdx:]
			sql = sql[:idx] + "REGEXP_SPLIT_TO_TABLE" + sql[opIdx:]
			continue
		}
		
		clIdx := findMatchingParenthesis(sql, opIdx)
		if clIdx == -1 {
			break
		}
		
		args := sql[opIdx+1 : clIdx]
		
		asIdx := clIdx + 1
		for asIdx < len(sql) && (sql[asIdx] == ' ' || sql[asIdx] == '\t' || sql[asIdx] == '\n' || sql[asIdx] == '\r') {
			asIdx++
		}
		
		alias := ""
		asEnd := asIdx
		if asIdx < len(sql) && strings.ToLower(sql[asIdx:min(asIdx+3, len(sql))]) == "as " {
			asEnd = asIdx + 3
			for asEnd < len(sql) && (sql[asEnd] == ' ' || sql[asEnd] == '\t' || sql[asEnd] == '\n' || sql[asEnd] == '\r') {
				asEnd++
			}
			aliasStart := asEnd
			for asEnd < len(sql) && isAlphaNumeric(sql[asEnd]) {
				asEnd++
			}
			alias = sql[aliasStart:asEnd]
		} else {
			aliasStart := asIdx
			for asEnd < len(sql) && isAlphaNumeric(sql[asEnd]) {
				asEnd++
			}
			alias = sql[aliasStart:asEnd]
		}
		
		if alias != "" {
			replacement := fmt.Sprintf("(SELECT value AS %s FROM json_each(minipsql_regexp_split_to_json(%s))) AS %s", alias, args, alias)
			sql = sql[:idx] + replacement + sql[asEnd:]
			lower = strings.ToLower(sql)
		} else {
			lower = lower[:idx] + "REGEXP_SPLIT_TO_TABLE" + lower[opIdx:]
			sql = sql[:idx] + "REGEXP_SPLIT_TO_TABLE" + sql[opIdx:]
		}
	}
	
	lower = strings.ToLower(sql)
	for {
		idx := strings.Index(lower, "select regexp_matches")
		if idx == -1 {
			break
		}
		
		opIdx := idx + len("select regexp_matches")
		for opIdx < len(sql) && (sql[opIdx] == ' ' || sql[opIdx] == '\t' || sql[opIdx] == '\n' || sql[opIdx] == '\r') {
			opIdx++
		}
		if opIdx >= len(sql) || sql[opIdx] != '(' {
			lower = lower[:idx] + "SELECT REGEXP_MATCHES" + lower[opIdx:]
			sql = sql[:idx] + "SELECT REGEXP_MATCHES" + sql[opIdx:]
			continue
		}
		
		clIdx := findMatchingParenthesis(sql, opIdx)
		if clIdx == -1 {
			break
		}
		
		args := sql[opIdx+1 : clIdx]
		
		endIdx := clIdx + 1
		for endIdx < len(sql) && (sql[endIdx] == ' ' || sql[endIdx] == '\t' || sql[endIdx] == '\n' || sql[endIdx] == '\r') {
			endIdx++
		}
		hasSemicolon := false
		if endIdx < len(sql) && sql[endIdx] == ';' {
			hasSemicolon = true
			endIdx++
		}
		
		suffix := ""
		if hasSemicolon {
			suffix = ";"
		}
		
		replacement := fmt.Sprintf("SELECT value FROM json_each(minipsql_regexp_matches_to_json(%s))%s", args, suffix)
		sql = sql[:idx] + replacement + sql[endIdx:]
		lower = strings.ToLower(sql)
	}
	
	return sql
}

func findMatchingParenthesis(s string, start int) int {
	depth := 0
	inQuote := false
	for i := start; i < len(s); i++ {
		if s[i] == '\'' {
			if inQuote && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func isAlphaNumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func rewriteOverlay(sql string) string {
	lower := strings.ToLower(sql)
	for {
		idx := indexOutsideQuotes(lower, "overlay")
		if idx == -1 {
			break
		}
		
		if idx > 0 && isIdentByte(sql[idx-1]) {
			lower = lower[:idx] + "OVERLAY" + lower[idx+len("overlay"):]
			continue
		}
		if idx+len("overlay") < len(sql) && isIdentByte(sql[idx+len("overlay")]) {
			lower = lower[:idx] + "OVERLAY" + lower[idx+len("overlay"):]
			continue
		}
		
		opIdx := idx + len("overlay")
		for opIdx < len(sql) && (sql[opIdx] == ' ' || sql[opIdx] == '\t' || sql[opIdx] == '\n' || sql[opIdx] == '\r') {
			opIdx++
		}
		if opIdx >= len(sql) || sql[opIdx] != '(' {
			lower = lower[:idx] + "OVERLAY" + lower[idx+len("overlay"):]
			continue
		}
		
		clIdx := findMatchingParenthesis(sql, opIdx)
		if clIdx == -1 {
			break
		}
		
		body := sql[opIdx+1 : clIdx]
		
		placingIdx := indexKeywordOutsideQuotesAndParens(body, "placing")
		fromIdx := indexKeywordOutsideQuotesAndParens(body, "from")
		forIdx := indexKeywordOutsideQuotesAndParens(body, "for")
		
		if placingIdx == -1 || fromIdx == -1 {
			lower = lower[:idx] + "OVERLAY" + lower[idx+len("overlay"):]
			continue
		}
		
		str := strings.TrimSpace(body[:placingIdx])
		newstr := strings.TrimSpace(body[placingIdx+len("placing") : fromIdx])
		
		var start, count string
		if forIdx != -1 && forIdx > fromIdx {
			start = strings.TrimSpace(body[fromIdx+len("from") : forIdx])
			count = strings.TrimSpace(body[forIdx+len("for"):])
		} else {
			start = strings.TrimSpace(body[fromIdx+len("from"):])
		}
		
		var replacement string
		if count != "" {
			replacement = fmt.Sprintf("minipsql_overlay(%s, %s, %s, %s)", str, newstr, start, count)
		} else {
			replacement = fmt.Sprintf("minipsql_overlay(%s, %s, %s)", str, newstr, start)
		}
		
		sql = sql[:idx] + replacement + sql[clIdx+1:]
		lower = strings.ToLower(sql)
	}
	return sql
}

func indexKeywordOutsideQuotesAndParens(s string, keyword string) int {
	lower := strings.ToLower(s)
	kLen := len(keyword)
	depth := 0
	inQuote := false
	for i := 0; i <= len(s)-kLen; i++ {
		if s[i] == '\'' {
			if inQuote && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
		}
		if depth == 0 && lower[i:i+kLen] == keyword {
			beforeOK := i == 0 || isWhitespaceOrOperator(s[i-1])
			after := i + kLen
			afterOK := after >= len(s) || isWhitespaceOrOperator(s[after])
			if beforeOK && afterOK {
				return i
			}
		}
	}
	return -1
}

func isWhitespaceOrOperator(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' || c == '(' || c == ')'
}

func rewriteStringContinuation(sql string) string {
	var b strings.Builder
	i := 0
	for i < len(sql) {
		if sql[i] == '\'' {
			strStart := i
			i++
			for i < len(sql) {
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			strContent := sql[strStart:i]
			
			for {
				nextIdx := i
				for nextIdx < len(sql) && unicode.IsSpace(rune(sql[nextIdx])) {
					nextIdx++
				}
				if nextIdx < len(sql) && sql[nextIdx] == '\'' {
					secStart := nextIdx
					nextIdx++
					for nextIdx < len(sql) {
						if sql[nextIdx] == '\'' {
							if nextIdx+1 < len(sql) && sql[nextIdx+1] == '\'' {
								nextIdx += 2
								continue
							}
							nextIdx++
							break
						}
						nextIdx++
					}
					secContent := sql[secStart:nextIdx]
					
					strVal := strContent[1 : len(strContent)-1]
					secVal := secContent[1 : len(secContent)-1]
					strContent = "'" + strVal + secVal + "'"
					i = nextIdx
				} else {
					break
				}
			}
			b.WriteString(strContent)
		} else if sql[i] == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			lineStart := i
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			b.WriteString(sql[lineStart:i])
		} else if sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			commentStart := i
			i += 2
			for i < len(sql) {
				if sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			b.WriteString(sql[commentStart:i])
		} else {
			b.WriteByte(sql[i])
			i++
		}
	}
	return b.String()
}

func rewriteToBinOctHex(sql string) string {
	lower := strings.ToLower(sql)
	for _, fn := range []string{"to_bin", "to_oct", "to_hex"} {
		for {
			idx := indexOutsideQuotes(lower, fn)
			if idx == -1 {
				break
			}
			
			if idx > 0 && isIdentByte(sql[idx-1]) {
				lower = lower[:idx] + strings.ToUpper(fn) + lower[idx+len(fn):]
				continue
			}
			if idx+len(fn) < len(sql) && isIdentByte(sql[idx+len(fn)]) {
				lower = lower[:idx] + strings.ToUpper(fn) + lower[idx+len(fn):]
				continue
			}
			
			opIdx := idx + len(fn)
			for opIdx < len(sql) && (sql[opIdx] == ' ' || sql[opIdx] == '\t' || sql[opIdx] == '\n' || sql[opIdx] == '\r') {
				opIdx++
			}
			if opIdx >= len(sql) || sql[opIdx] != '(' {
				lower = lower[:idx] + strings.ToUpper(fn) + lower[opIdx:]
				continue
			}
			
			clIdx := findMatchingParenthesis(sql, opIdx)
			if clIdx == -1 {
				break
			}
			
			body := sql[opIdx+1 : clIdx]
			bodyLower := strings.ToLower(body)
			
			isBig := strings.Contains(bodyLower, "bigint") || strings.Contains(bodyLower, "int8")
			
			var newFn string
			if isBig {
				newFn = "minipsql_" + fn + "_bigint"
			} else {
				newFn = "minipsql_" + fn + "_int"
			}
			
			sql = sql[:idx] + newFn + "(" + body + ")" + sql[clIdx+1:]
			lower = strings.ToLower(sql)
		}
	}
	return sql
}

func rewriteSubstring(sql string) string {
	lower := strings.ToLower(sql)
	for {
		idx := indexOutsideQuotes(lower, "substring")
		if idx == -1 {
			break
		}
		
		if idx > 0 && isIdentByte(sql[idx-1]) {
			lower = lower[:idx] + "SUBSTRING" + lower[idx+len("substring"):]
			continue
		}
		if idx+len("substring") < len(sql) && isIdentByte(sql[idx+len("substring")]) {
			lower = lower[:idx] + "SUBSTRING" + lower[idx+len("substring"):]
			continue
		}
		
		opIdx := idx + len("substring")
		for opIdx < len(sql) && (sql[opIdx] == ' ' || sql[opIdx] == '\t' || sql[opIdx] == '\n' || sql[opIdx] == '\r') {
			opIdx++
		}
		if opIdx >= len(sql) || sql[opIdx] != '(' {
			lower = lower[:idx] + "SUBSTRING" + lower[opIdx:]
			continue
		}
		
		clIdx := findMatchingParenthesis(sql, opIdx)
		if clIdx == -1 {
			break
		}
		
		body := sql[opIdx+1 : clIdx]
		
		similarIdx := indexKeywordOutsideQuotesAndParens(body, "similar")
		if similarIdx != -1 {
			val := strings.TrimSpace(body[:similarIdx])
			tail := body[similarIdx+len("similar"):]
			escapeIdx := indexKeywordOutsideQuotesAndParens(tail, "escape")
			var pat, esc string
			if escapeIdx != -1 {
				pat = strings.TrimSpace(tail[:escapeIdx])
				esc = strings.TrimSpace(tail[escapeIdx+len("escape"):])
			} else {
				pat = strings.TrimSpace(tail)
				esc = "'\\'"
			}
			rewritten := "minipsql_similar_substring(" + val + ", " + pat + ", " + esc + ")"
			sql = sql[:idx] + rewritten + sql[clIdx+1:]
			lower = strings.ToLower(sql)
			continue
		}
		
		fromIdx := indexKeywordOutsideQuotesAndParens(body, "from")
		forIdx := indexKeywordOutsideQuotesAndParens(body, "for")
		
		if fromIdx != -1 && forIdx != -1 && forIdx > fromIdx {
			str := strings.TrimSpace(body[:fromIdx])
			patOrStart := strings.TrimSpace(body[fromIdx+len("from") : forIdx])
			escOrLen := strings.TrimSpace(body[forIdx+len("for"):])
			
			isSimilarRegexp := strings.Contains(patOrStart, `#"`) || (strings.HasPrefix(escOrLen, "'") && strings.HasSuffix(escOrLen, "'") && len(escOrLen) == 3)
			if isSimilarRegexp {
				rewritten := "minipsql_similar_substring(" + str + ", " + patOrStart + ", " + escOrLen + ")"
				sql = sql[:idx] + rewritten + sql[clIdx+1:]
				lower = strings.ToLower(sql)
				continue
			}
		}
		
		if fromIdx == -1 {
			// SUBSTRING(str FOR len) — no FROM, only FOR
			if forIdx != -1 {
				str := strings.TrimSpace(body[:forIdx])
				length := strings.TrimSpace(body[forIdx+len("for"):])
				rewritten := "substr(" + str + ", 1, " + length + ")"
				sql = sql[:idx] + rewritten + sql[clIdx+1:]
			} else {
				sql = sql[:idx] + "substr(" + body + ")" + sql[clIdx+1:]
			}
			lower = strings.ToLower(sql)
			continue
		}
		
		str := strings.TrimSpace(body[:fromIdx])
		var start, length string
		if forIdx != -1 && forIdx > fromIdx {
			start = strings.TrimSpace(body[fromIdx+len("from") : forIdx])
			length = strings.TrimSpace(body[forIdx+len("for"):])
		} else {
			start = strings.TrimSpace(body[fromIdx+len("from"):])
		}
		
		var rewritten string
		if length != "" {
			rewritten = "substr(" + str + ", " + start + ", " + length + ")"
		} else {
			rewritten = "substr(" + str + ", " + start + ")"
		}
		
		sql = sql[:idx] + rewritten + sql[clIdx+1:]
		lower = strings.ToLower(sql)
	}
	return sql
}

func rewriteTrim(sql string) string {
	lower := strings.ToLower(sql)
	for {
		idx := indexOutsideQuotes(lower, "trim")
		if idx == -1 {
			break
		}
		
		if idx > 0 && isIdentByte(sql[idx-1]) {
			lower = lower[:idx] + "TRIM" + lower[idx+len("trim"):]
			continue
		}
		if idx+len("trim") < len(sql) && isIdentByte(sql[idx+len("trim")]) {
			lower = lower[:idx] + "TRIM" + lower[idx+len("trim"):]
			continue
		}
		
		opIdx := idx + len("trim")
		for opIdx < len(sql) && (sql[opIdx] == ' ' || sql[opIdx] == '\t' || sql[opIdx] == '\n' || sql[opIdx] == '\r') {
			opIdx++
		}
		if opIdx >= len(sql) || sql[opIdx] != '(' {
			lower = lower[:idx] + "TRIM" + lower[opIdx:]
			continue
		}
		
		clIdx := findMatchingParenthesis(sql, opIdx)
		if clIdx == -1 {
			break
		}
		
		body := sql[opIdx+1 : clIdx]
		
		fromIdx := indexKeywordOutsideQuotesAndParens(body, "from")
		if fromIdx == -1 {
			sql = sql[:idx] + "btrim(" + body + ")" + sql[clIdx+1:]
			lower = strings.ToLower(sql)
			continue
		}
		
		str := strings.TrimSpace(body[fromIdx+len("from"):])
		prefix := strings.TrimSpace(body[:fromIdx])
		
		direction := "both"
		chars := ""
		
		prefLower := strings.ToLower(prefix)
		if strings.HasPrefix(prefLower, "leading ") || prefLower == "leading" {
			direction = "leading"
			chars = strings.TrimSpace(prefix[len("leading"):])
		} else if strings.HasPrefix(prefLower, "trailing ") || prefLower == "trailing" {
			direction = "trailing"
			chars = strings.TrimSpace(prefix[len("trailing"):])
		} else if strings.HasPrefix(prefLower, "both ") || prefLower == "both" {
			direction = "both"
			chars = strings.TrimSpace(prefix[len("both"):])
		} else {
			direction = "both"
			chars = prefix
		}
		
		var rewritten string
		if direction == "leading" {
			if chars != "" {
				rewritten = "ltrim(" + str + ", " + chars + ")"
			} else {
				rewritten = "ltrim(" + str + ")"
			}
		} else if direction == "trailing" {
			if chars != "" {
				rewritten = "rtrim(" + str + ", " + chars + ")"
			} else {
				rewritten = "rtrim(" + str + ")"
			}
		} else {
			if chars != "" {
				rewritten = "btrim(" + str + ", " + chars + ")"
			} else {
				rewritten = "btrim(" + str + ")"
			}
		}
		
		sql = sql[:idx] + rewritten + sql[clIdx+1:]
		lower = strings.ToLower(sql)
	}
	return sql
}

func rewriteUnicodeLiterals(sql string) string {
	var b strings.Builder
	i := 0
	n := len(sql)
	
	makeError := func(msg, code, hint string) string {
		escMsg := strings.ReplaceAll(msg, "'", "''")
		escCode := strings.ReplaceAll(code, "'", "''")
		escHint := strings.ReplaceAll(hint, "'", "''")
		return "minipsql_error('" + escMsg + "', '" + escCode + "', '" + escHint + "')"
	}
	
	inSingle := false
	inDouble := false
	
	for i < n {
		if inSingle {
			if sql[i] == '\'' {
				if i+1 < n && sql[i+1] == '\'' {
					b.WriteByte('\'')
					b.WriteByte('\'')
					i += 2
					continue
				}
				inSingle = false
			}
			b.WriteByte(sql[i])
			i++
			continue
		}
		if inDouble {
			if sql[i] == '"' {
				if i+1 < n && sql[i+1] == '"' {
					b.WriteByte('"')
					b.WriteByte('"')
					i += 2
					continue
				}
				inDouble = false
			}
			b.WriteByte(sql[i])
			i++
			continue
		}
		
		// Check for U&' or u&' or U&" or u&"
		if i+2 < n && (sql[i] == 'U' || sql[i] == 'u') && sql[i+1] == '&' && (sql[i+2] == '\'' || sql[i+2] == '"') {
			quoteChar := sql[i+2]
			startQuoteIdx := i + 2
			endQuoteIdx := -1
			j := startQuoteIdx + 1
			for j < n {
				if sql[j] == quoteChar {
					if j+1 < n && sql[j+1] == quoteChar {
						j += 2
						continue
					}
					endQuoteIdx = j
					break
				}
				j++
			}
			if endQuoteIdx == -1 {
				b.WriteByte(sql[i])
				i++
				continue
			}
			
			rawBody := sql[startQuoteIdx+1 : endQuoteIdx]
			
			// Check if followed by UESCAPE 'x'
			k := endQuoteIdx + 1
			for k < n && (sql[k] == ' ' || sql[k] == '\t' || sql[k] == '\n' || sql[k] == '\r') {
				k++
			}
			
			escapeChar := '\\'
			hasUEscape := false
			uescapeError := ""
			
			if k+7 < n && strings.ToLower(sql[k:k+7]) == "uescape" {
				k += 7
				for k < n && (sql[k] == ' ' || sql[k] == '\t' || sql[k] == '\n' || sql[k] == '\r') {
					k++
				}
				if k < n && sql[k] == '\'' {
					eq := k + 1
					for eq < n && sql[eq] != '\'' {
						eq++
					}
					if eq < n && sql[eq] == '\'' {
						escStr := sql[k+1 : eq]
						if len(escStr) == 1 {
							r := rune(escStr[0])
							isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
							if isHex || r == '+' || r == '\'' || r == '"' || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
								uescapeError = "invalid_escape"
							} else {
								escapeChar = r
							}
						} else {
							uescapeError = "invalid_escape"
						}
						k = eq + 1
						hasUEscape = true
					} else {
						b.WriteString("near \"UESCAPE\": syntax error")
						i = k
						continue
					}
				} else {
					// e.g. UESCAPE +
					b.WriteString(makeError("UESCAPE must be followed by a simple string literal at or near \"+\"", "42601", ""))
					i = k + 1
					if i < n && sql[i] == ';' {
						b.WriteByte(';')
						i++
					}
					continue
				}
			}
			
			if uescapeError == "invalid_escape" {
				b.WriteString(makeError("invalid Unicode escape character", "22025", ""))
				i = k
				continue
			}
			
			decoded, errStr, errHint := decodeUnicodeEscape(rawBody, escapeChar)
			if errStr != "" {
				b.WriteString(makeError(errStr, "22025", errHint))
			} else {
				if quoteChar == '\'' {
					b.WriteByte('\'')
					b.WriteString(strings.ReplaceAll(decoded, "'", "''"))
					b.WriteByte('\'')
				} else {
					b.WriteByte('"')
					b.WriteString(strings.ReplaceAll(decoded, "\"", "\"\""))
					b.WriteByte('"')
				}
			}
			if hasUEscape {
				i = k
			} else {
				i = endQuoteIdx + 1
			}
			continue
		}
		
		if sql[i] == '\'' {
			inSingle = true
		} else if sql[i] == '"' {
			inDouble = true
		}
		b.WriteByte(sql[i])
		i++
	}
	return b.String()
}

func decodeUnicodeEscape(s string, esc rune) (string, string, string) {
	var b strings.Builder
	runes := []rune(s)
	n := len(runes)
	hint := "Unicode escapes must be " + string(esc) + "XXXX or " + string(esc) + "+XXXXXX."

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == esc {
			if i+1 < n && runes[i+1] == esc {
				b.WriteRune(esc)
				i++
				continue
			}
			if i+1 < n && runes[i+1] == '+' {
				if i+7 < n {
					hexStr := string(runes[i+2 : i+8])
					val, err := strconv.ParseUint(hexStr, 16, 32)
					if err != nil {
						return "", "invalid Unicode escape", hint
					}
					if val > 0x10FFFF {
						return "", fmt.Sprintf("invalid Unicode code point: %s", hexStr), ""
					}
					if val >= 0xD800 && val <= 0xDFFF {
						return "", "invalid Unicode surrogate pair", ""
					}
					b.WriteRune(rune(val))
					i += 7
					continue
				} else {
					return "", "invalid Unicode escape", hint
				}
			}
			if i+4 < n {
				hexStr := string(runes[i+1 : i+5])
				val, err := strconv.ParseUint(hexStr, 16, 32)
				if err != nil {
					return "", "invalid Unicode escape", hint
				}
				if val >= 0xD800 && val <= 0xDBFF {
					if i+9 < n && runes[i+5] == esc && runes[i+6] != '+' {
						nextHex := string(runes[i+6 : i+10])
						nextVal, err := strconv.ParseUint(nextHex, 16, 32)
						if err == nil && nextVal >= 0xDC00 && nextVal <= 0xDFFF {
							cp := 0x10000 + ((val - 0xD800) << 10) + (nextVal - 0xDC00)
							b.WriteRune(rune(cp))
							i += 9
							continue
						}
					}
					return "", "invalid Unicode surrogate pair", ""
				}
				if val >= 0xDC00 && val <= 0xDFFF {
					return "", "invalid Unicode surrogate pair", ""
				}
				b.WriteRune(rune(val))
				i += 4
				continue
			} else {
				return "", "invalid Unicode escape", hint
			}
		} else {
			b.WriteRune(r)
		}
	}
	return b.String(), "", ""
}

// rewriteStringAgg rewrites PostgreSQL string_agg(expr, sep [ORDER BY ...]) to
// SQLite group_concat(expr, sep), discarding any ORDER BY inside the aggregate.
func rewriteStringAgg(sql string) string {
	return rewriteAggFunc(sql, "string_agg", func(inner string) string {
		// inner is everything between the outer parens
		// strip ORDER BY clause inside aggregate if present
		lower := strings.ToLower(inner)
		orderIdx := findOrderByOutsideParens(lower)
		if orderIdx >= 0 {
			inner = inner[:orderIdx]
		}
		return "group_concat(" + strings.TrimSpace(inner) + ")"
	})
}

// rewriteArrayAgg rewrites PostgreSQL array_agg(expr [ORDER BY ...]) to
// SQLite json_group_array(expr), discarding ORDER BY inside aggregate.
func rewriteArrayAgg(sql string) string {
	return rewriteAggFunc(sql, "array_agg", func(inner string) string {
		lower := strings.ToLower(inner)
		orderIdx := findOrderByOutsideParens(lower)
		if orderIdx >= 0 {
			inner = inner[:orderIdx]
		}
		return "json_group_array(" + strings.TrimSpace(inner) + ")"
	})
}

// rewriteAggFunc is a generic helper to rewrite aggregate function calls.
func rewriteAggFunc(sql, funcName string, transform func(string) string) string {
	lower := strings.ToLower(sql)
	var result strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], funcName+"(")
		if idx == -1 {
			break
		}
		idx += pos
		// Check word boundary before funcName
		if idx > 0 && isIdentByte(sql[idx-1]) {
			pos = idx + 1
			continue
		}
		parenStart := idx + len(funcName)
		// Find matching closing paren
		depth := 0
		i := parenStart
		inSingle := false
		inDouble := false
		for i < len(sql) {
			c := sql[i]
			if inSingle {
				if c == '\'' && i+1 < len(sql) && sql[i+1] == '\'' {
					i += 2
					continue
				}
				if c == '\'' {
					inSingle = false
				}
				i++
				continue
			}
			if inDouble {
				if c == '"' && i+1 < len(sql) && sql[i+1] == '"' {
					i += 2
					continue
				}
				if c == '"' {
					inDouble = false
				}
				i++
				continue
			}
			if c == '\'' {
				inSingle = true
			} else if c == '"' {
				inDouble = true
			} else if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
			i++
		}
		if depth != 0 {
			break
		}
		inner := sql[parenStart+1 : i]
		result.WriteString(sql[pos:idx])
		result.WriteString(transform(inner))
		pos = i + 1
		lower = strings.ToLower(sql)
	}
	result.WriteString(sql[pos:])
	return result.String()
}

// findOrderByOutsideParens finds "order by" keyword outside of parentheses.
func findOrderByOutsideParens(lower string) int {
	depth := 0
	inSingle := false
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if inSingle {
			if c == '\'' && i+1 < len(lower) && lower[i+1] == '\'' {
				i++
				continue
			}
			if c == '\'' {
				inSingle = false
			}
			continue
		}
		if c == '\'' {
			inSingle = true
			continue
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			depth--
		} else if depth == 0 && i+8 <= len(lower) && lower[i:i+8] == "order by" {
			// check word boundaries
			leftOK := i == 0 || lower[i-1] == ' ' || lower[i-1] == '\t' || lower[i-1] == '\n'
			rightOK := i+8 == len(lower) || lower[i+8] == ' ' || lower[i+8] == '\t' || lower[i+8] == '\n'
			if leftOK && rightOK {
				return i
			}
		}
	}
	return -1
}

func rewriteKeywords(sql string) string {
	var b strings.Builder
	n := len(sql)
	inSingle := false
	inDouble := false
	
	i := 0
	for i < n {
		if inSingle {
			if sql[i] == '\'' {
				if i+1 < n && sql[i+1] == '\'' {
					b.WriteByte('\'')
					b.WriteByte('\'')
					i += 2
					continue
				}
				inSingle = false
			}
			b.WriteByte(sql[i])
			i++
			continue
		}
		if inDouble {
			if sql[i] == '"' {
				if i+1 < n && sql[i+1] == '"' {
					b.WriteByte('"')
					b.WriteByte('"')
					i += 2
					continue
				}
				inDouble = false
			}
			b.WriteByte(sql[i])
			i++
			continue
		}
		
		if sql[i] == '\'' {
			inSingle = true
			b.WriteByte(sql[i])
			i++
			continue
		}
		if sql[i] == '"' {
			inDouble = true
			b.WriteByte(sql[i])
			i++
			continue
		}
		
		if i+7 <= n && strings.ToLower(sql[i:i+7]) == "unknown" {
			leftBoundary := i == 0 || !isIdentByte(sql[i-1])
			rightBoundary := i+7 == n || !isIdentByte(sql[i+7])
			if leftBoundary && rightBoundary {
				b.WriteString("NULL")
				i += 7
				continue
			}
		}
		// Rewrite PostgreSQL session/user keywords to function calls
		for _, kw := range []struct{ word, fn string }{
			{"current_user", "current_user()"},
			{"session_user", "session_user()"},
			{"current_role", "current_role()"},
			{"current_schema", "current_schema()"},
			{"current_database", "current_database()"},
			{"current_catalog", "current_catalog()"},
		} {
			if i+len(kw.word) <= n && strings.ToLower(sql[i:i+len(kw.word)]) == kw.word {
				// ensure word boundary
				leftOk := i == 0 || !isIdentByte(sql[i-1])
				rightEnd := i + len(kw.word)
				rightOk := rightEnd == n || !isIdentByte(sql[rightEnd])
				// don't rewrite if already followed by '('
				notFunc := rightEnd >= n || sql[rightEnd] != '('
				if leftOk && rightOk && notFunc {
					b.WriteString(kw.fn)
					i += len(kw.word)
					goto nextChar
				}
			}
		}
		
		b.WriteByte(sql[i])
		i++
	nextChar:
	}
	return b.String()
}

func rewriteValuesAlias(sql string) string {
	lower := strings.ToLower(sql)
	for {
		idx := indexOutsideQuotes(lower, "values")
		if idx == -1 {
			break
		}
		
		if idx > 0 && isIdentByte(sql[idx-1]) {
			lower = lower[:idx] + "VALUES" + lower[idx+len("values"):]
			continue
		}
		if idx+len("values") < len(sql) && isIdentByte(sql[idx+len("values")]) {
			lower = lower[:idx] + "VALUES" + lower[idx+len("values"):]
			continue
		}
		
		i := idx + len("values")
		for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
			i++
		}
		if i >= len(sql) || sql[i] != '(' {
			lower = lower[:idx] + "VALUES" + lower[i:]
			continue
		}
		
		var rows []string
		startRowIdx := i
		depth := 0
		inSingle := false
		inDouble := false
		
		success := false
		
		for i < len(sql) {
			c := sql[i]
			if inSingle {
				if c == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						i += 2
						continue
					}
					inSingle = false
				}
				i++
				continue
			}
			if inDouble {
				if c == '"' {
					if i+1 < len(sql) && sql[i+1] == '"' {
						i += 2
						continue
					}
					inDouble = false
				}
				i++
				continue
			}
			
			switch c {
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					rowContent := sql[startRowIdx+1 : i]
					rows = append(rows, rowContent)
					i++
					for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
						i++
					}
					if i < len(sql) && sql[i] == ',' {
						i++
						for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
							i++
						}
						if i < len(sql) && sql[i] == '(' {
							startRowIdx = i
							depth = 1
						} else {
							goto fail
						}
					} else {
						success = true
						goto doneParse
					}
				}
			}
			i++
		}
	fail:
		lower = lower[:idx] + "VALUES" + lower[idx+len("values"):]
		continue
		
	doneParse:
		if !success || len(rows) == 0 {
			lower = lower[:idx] + "VALUES" + lower[idx+len("values"):]
			continue
		}
		
		hasOuterParen := false
		if i < len(sql) && sql[i] == ')' {
			hasOuterParen = true
			i++
		}
		
		for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
			i++
		}
		
		hasAS := false
		if i+2 < len(sql) && strings.ToLower(sql[i:i+2]) == "as" && !isIdentByte(sql[i+2]) {
			hasAS = true
			i += 2
			for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
				i++
			}
		}
		
		if i >= len(sql) || !isIdentStartByte(sql[i]) {
			lower = lower[:idx] + "VALUES" + lower[idx+len("values"):]
			continue
		}
		
		aliasStart := i
		for i < len(sql) && isIdentByte(sql[i]) {
			i++
		}
		aliasName := sql[aliasStart:i]
		
		for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
			i++
		}
		
		if i >= len(sql) || sql[i] != '(' {
			lower = lower[:idx] + "VALUES" + lower[idx+len("values"):]
			continue
		}
		
		i++
		for i < len(sql) && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
			i++
		}
		
		colStart := i
		for i < len(sql) && sql[i] != ')' {
			i++
		}
		if i >= len(sql) || sql[i] != ')' {
			lower = lower[:idx] + "VALUES" + lower[idx+len("values"):]
			continue
		}
		colNamesStr := strings.TrimSpace(sql[colStart:i])
		i++
		
		colParts := strings.Split(colNamesStr, ",")
		var colNames []string
		for _, col := range colParts {
			colNames = append(colNames, strings.TrimSpace(col))
		}
		
		var newSelects []string
		for rIdx, r := range rows {
			cols := splitColumnsOutsideQuotesAndParens(r)
			if len(cols) != len(colNames) {
				goto fail
			}
			
			var colExprs []string
			for cIdx, colVal := range cols {
				if rIdx == 0 {
					colExprs = append(colExprs, colVal+" AS "+colNames[cIdx])
				} else {
					colExprs = append(colExprs, colVal)
				}
			}
			newSelects = append(newSelects, "SELECT "+strings.Join(colExprs, ", "))
		}
		
		rewritten := "(" + strings.Join(newSelects, " UNION ALL ") + ") "
		if hasAS {
			rewritten += "AS "
		}
		rewritten += aliasName

		// If the VALUES list was wrapped in outer parens, replace them too
		replaceStart := idx
		if hasOuterParen {
			// Look backwards for the matching '(' before 'values'
			j := idx - 1
			for j >= 0 && (sql[j] == ' ' || sql[j] == '\t') { j-- }
			if j >= 0 && sql[j] == '(' {
				replaceStart = j
			}
		}

		sql = sql[:replaceStart] + rewritten + sql[i:]
		lower = strings.ToLower(sql)
	}
	return sql
}

func isIdentStartByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func splitColumnsOutsideQuotesAndParens(s string) []string {
	var res []string
	depth := 0
	inSingle := false
	inDouble := false
	startIdx := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inSingle {
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '"' {
				if i+1 < len(s) && s[i+1] == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				res = append(res, strings.TrimSpace(s[startIdx:i]))
				startIdx = i + 1
			}
		}
	}
	res = append(res, strings.TrimSpace(s[startIdx:]))
	return res
}

func rewriteTableColumnAlias(sql string) string {
	return tableColumnAliasRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := tableColumnAliasRE.FindStringSubmatch(m)
		tableName := sub[1]
		aliasName := sub[2]
		colsStr := sub[3]

		colParts := strings.Split(colsStr, ",")
		numCols := len(colParts)

		if numCols == 1 {
			colName := strings.TrimSpace(colParts[0])
			return fmt.Sprintf("FROM (SELECT f1 AS %s FROM %s) %s", colName, tableName, aliasName)
		}
		// Strip column alias list — just use table alias without column renaming
		return fmt.Sprintf("FROM %s %s", tableName, aliasName)
	})
}

// tableColAliasSQLKeywords is the set of SQL keywords that cannot be table aliases.
var tableColAliasSQLKeywords = map[string]bool{
	"WHERE": true, "ON": true, "SET": true, "HAVING": true, "ORDER": true,
	"GROUP": true, "LIMIT": true, "OFFSET": true, "UNION": true, "EXCEPT": true,
	"INTERSECT": true, "RETURNING": true, "INTO": true, "LEFT": true, "RIGHT": true,
	"INNER": true, "OUTER": true, "CROSS": true, "FULL": true, "NATURAL": true,
	"LATERAL": true, "NOT": true, "AND": true, "OR": true, "IS": true, "IN": true,
	"LIKE": true, "BETWEEN": true, "CASE": true, "WHEN": true, "THEN": true,
	"ELSE": true, "END": true, "SELECT": true, "FROM": true, "JOIN": true,
	"AS": true, "WITH": true, "BY": true, "FETCH": true, "ROWS": true, "ROW": true,
}

// rewriteGenericTableColAlias strips column alias lists from table references like
// "FROM table alias (col1, col2, ...)" → "FROM table alias"
// PostgreSQL allows renaming columns via this syntax but SQLite does not.
func rewriteGenericTableColAlias(sql string) string {
	if !strings.Contains(sql, "(") {
		return sql
	}
	return genericTableColAliasRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := genericTableColAliasRE.FindStringSubmatch(m)
		tableName := sub[1]
		aliasName := sub[2]
		// Skip if aliasName is a SQL keyword (false positive: "FROM t WHERE (a,b) NOT IN ...")
		if tableColAliasSQLKeywords[strings.ToUpper(aliasName)] {
			return m
		}
		// Preserve the leading FROM/JOIN keyword with its original spacing
		upperM := strings.ToUpper(m)
		var sep string
		if idx := strings.Index(upperM, "FROM"); idx >= 0 && (idx == 0 || m[idx-1] == ' ' || m[idx-1] == '\n' || m[idx-1] == '\t') {
			sep = m[:idx] + "FROM "
		} else if idx := strings.Index(upperM, "JOIN"); idx >= 0 {
			sep = m[:idx] + "JOIN "
		} else {
			sep = "FROM "
		}
		return sep + tableName + " " + aliasName
	})
}

// rewriteSchemaPrefix strips public., pg_temp., pg_catalog. schema prefixes
// that SQLite does not understand.
func rewriteSchemaPrefix(sql string) string {
	return schemaPrefixRE.ReplaceAllString(sql, "$1")
}

// rewriteViewOptions strips WITH (option=value) from CREATE VIEW statements.
func rewriteViewOptions(sql string) string {
	return createViewWithOptsRE.ReplaceAllString(sql, "$1")
}

// rewriteTableWithOptions strips WITH (option=value) from CREATE TABLE ... AS SELECT.
func rewriteTableWithOptions(sql string) string {
	return createTableWithOptsRE.ReplaceAllString(sql, "$1")
}

// rewriteArrayIndex converts PostgreSQL 1-based array index access to json_extract.
//   identifier[N]         →  json_extract(identifier, '$[N-1]')
//   identifier[expr]      →  json_extract(identifier, '$[' || max(0, (expr) - 1) || ']')
func rewriteArrayIndex(sql string) string {
	if !strings.Contains(sql, "[") {
		return sql
	}
	// First handle numeric indices (most common, cheapest)
	out := arrayIndexRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := arrayIndexRE.FindStringSubmatch(m)
		name := sub[1]
		idx, _ := strconv.Atoi(sub[2])
		return fmt.Sprintf("json_extract(%s, '$[%d]')", name, idx-1)
	})
	// Then handle variable/expression indices: col[expr] → json_extract(col, '$['||(expr-1)||']')
	out = arrayVarIndexRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := arrayVarIndexRE.FindStringSubmatch(m)
		name := sub[1]
		expr := sub[2]
		return fmt.Sprintf("json_extract(%s, '$['||max(0,(%s)-1)||']')", name, expr)
	})
	return out
}

// rewriteUnnest converts FROM unnest(arr) [AS alias[(col)]] to FROM json_each(arr) AS alias.
// PostgreSQL's unnest(array) SRF is emulated with SQLite's json_each table-valued function.
// Only converts unnest() that appears in a FROM/JOIN clause context.
func rewriteUnnest(sql string) string {
	lower := strings.ToLower(sql)
	for {
		idx := strings.Index(lower, "unnest")
		if idx == -1 {
			break
		}
		// Check word boundary before and after
		if idx > 0 && isIdentByte(sql[idx-1]) {
			lower = lower[:idx] + "UNNEST" + lower[idx+6:]
			continue
		}
		if idx+6 < len(sql) && isIdentByte(sql[idx+6]) {
			lower = lower[:idx] + "UNNEST" + lower[idx+6:]
			continue
		}
		// Check that this unnest is preceded by FROM, JOIN, or COMMA in FROM context
		// by scanning backwards for a FROM/JOIN/CROSS JOIN/LEFT JOIN keyword
		before := strings.TrimRight(sql[:idx], " \t\n")
		beforeLower := strings.ToLower(before)
		isFromContext := strings.HasSuffix(beforeLower, "from") ||
			strings.HasSuffix(beforeLower, "join") ||
			strings.HasSuffix(beforeLower, ",")
		if !isFromContext {
			lower = lower[:idx] + "UNNEST" + lower[idx+6:]
			continue
		}
		j := idx + 6
		for j < len(sql) && (sql[j] == ' ' || sql[j] == '\t') {
			j++
		}
		if j >= len(sql) || sql[j] != '(' {
			lower = lower[:idx] + "UNNEST" + lower[idx+6:]
			continue
		}
		// Find matching close paren
		end := findMatchingParen(sql, j)
		if end < 0 {
			break
		}
		inner := sql[j+1 : end]
		// If inner contains minipsql_array (from ARRAY[] constructor), convert to json_array
		// so that json_each() receives valid JSON instead of PostgreSQL {a,b} array syntax.
		inner = strings.ReplaceAll(inner, "minipsql_array(", "json_array(")
		// Scan for optional WITH ORDINALITY, AS alias[(col)] after the closing paren
		after := end + 1
		rawAfter := after // position before skipping whitespace — used as aliasEnd when no alias found
		for after < len(sql) && (sql[after] == ' ' || sql[after] == '\t' || sql[after] == '\n' || sql[after] == '\r') {
			after++
		}
		withOrdinality := false
		alias := ""
		colNames := []string{}
		aliasEnd := rawAfter
		// Check for WITH ORDINALITY
		if after+15 <= len(sql) && strings.EqualFold(sql[after:after+15], "with ordinality") {
			withOrdinality = true
			aliasEnd = after + 15
			for aliasEnd < len(sql) && (sql[aliasEnd] == ' ' || sql[aliasEnd] == '\t' || sql[aliasEnd] == '\n' || sql[aliasEnd] == '\r') {
				aliasEnd++
			}
			after = aliasEnd
		}
		// Parse AS alias or bare alias
		asKeyword := after+2 <= len(sql) && strings.EqualFold(sql[after:after+2], "as") &&
			(after+2 >= len(sql) || sql[after+2] == ' ' || sql[after+2] == '\t' || sql[after+2] == '\n' || sql[after+2] == '\r')
		if after < len(sql) && asKeyword {
			aliasEnd = after + 2
			for aliasEnd < len(sql) && (sql[aliasEnd] == ' ' || sql[aliasEnd] == '\t' || sql[aliasEnd] == '\n' || sql[aliasEnd] == '\r') {
				aliasEnd++
			}
			aliasStart := aliasEnd
			for aliasEnd < len(sql) && isAlphaNumeric(sql[aliasEnd]) {
				aliasEnd++
			}
			if aliasEnd > aliasStart {
				alias = sql[aliasStart:aliasEnd]
			}
			// Parse optional column alias list
			if aliasEnd < len(sql) && sql[aliasEnd] == '(' {
				parenEnd := findMatchingParen(sql, aliasEnd)
				if parenEnd >= 0 {
					colList := sql[aliasEnd+1 : parenEnd]
					for _, c := range strings.Split(colList, ",") {
						colNames = append(colNames, strings.TrimSpace(c))
					}
					aliasEnd = parenEnd + 1
				}
			}
		} else if after < len(sql) && isAlphaNumeric(sql[after]) &&
			!startsWithKeyword(sql[after:], "ORDER") &&
			!startsWithKeyword(sql[after:], "WHERE") &&
			!startsWithKeyword(sql[after:], "GROUP") &&
			!startsWithKeyword(sql[after:], "LIMIT") &&
			!startsWithKeyword(sql[after:], "HAVING") &&
			!startsWithKeyword(sql[after:], "UNION") &&
			!startsWithKeyword(sql[after:], "ON") &&
			!startsWithKeyword(sql[after:], "LEFT") &&
			!startsWithKeyword(sql[after:], "RIGHT") &&
			!startsWithKeyword(sql[after:], "INNER") &&
			!startsWithKeyword(sql[after:], "CROSS") &&
			!startsWithKeyword(sql[after:], "JOIN") &&
			!startsWithKeyword(sql[after:], "EXCEPT") &&
			!startsWithKeyword(sql[after:], "INTERSECT") {
			aliasStart := after
			for aliasEnd < len(sql) && isAlphaNumeric(sql[aliasEnd]) {
				aliasEnd++
			}
			if aliasEnd > aliasStart {
				alias = sql[aliasStart:aliasEnd]
			}
			if aliasEnd < len(sql) && sql[aliasEnd] == '(' {
				parenEnd := findMatchingParen(sql, aliasEnd)
				if parenEnd >= 0 {
					colList := sql[aliasEnd+1 : parenEnd]
					for _, c := range strings.Split(colList, ",") {
						colNames = append(colNames, strings.TrimSpace(c))
					}
					aliasEnd = parenEnd + 1
				}
			}
		}
		if alias == "" {
			alias = "unnest_t"
		}
		// Build replacement:
		// If column names provided, wrap in subquery to rename json_each columns
		var replacement string
		if len(colNames) > 0 {
			valCol := colNames[0]
			if withOrdinality && len(colNames) >= 2 {
				ordCol := colNames[1]
				replacement = fmt.Sprintf("(SELECT value AS %s, key+1 AS %s FROM json_each(%s)) AS %s",
					valCol, ordCol, inner, alias)
			} else {
				replacement = fmt.Sprintf("(SELECT value AS %s FROM json_each(%s)) AS %s",
					valCol, inner, alias)
			}
		} else if withOrdinality {
			replacement = fmt.Sprintf("(SELECT value, key+1 AS ordinality FROM json_each(%s)) AS %s",
				inner, alias)
		} else {
			replacement = fmt.Sprintf("json_each(%s) AS %s", inner, alias)
		}
		sql = sql[:idx] + replacement + sql[aliasEnd:]
		lower = strings.ToLower(sql)
	}
	return sql
}

// rewriteArrayConcatStrip removes "|| ARRAY(SELECT ...)" from expressions like
// c.reloptions || array(select ...) → c.reloptions
// SQLite cannot handle identifier || (SELECT subquery) array-concat patterns.
func rewriteArrayConcatStrip(sql string) string {
	if !pgArrayConcatRE.MatchString(sql) {
		return sql
	}
	for {
		loc := pgArrayConcatRE.FindStringIndex(sql)
		if loc == nil {
			break
		}
		// loc[1]-1 points to the '(' after ARRAY
		openParen := loc[1] - 1
		// find the SELECT keyword inside
		k := openParen + 1
		for k < len(sql) && (sql[k] == ' ' || sql[k] == '\t' || sql[k] == '\n') {
			k++
		}
		if k+6 > len(sql) || !strings.EqualFold(sql[k:k+6], "select") {
			// not a SELECT subquery, skip
			break
		}
		// find matching close paren
		depth := 1
		end := openParen + 1
		inS := false
		for end < len(sql) && depth > 0 {
			if inS {
				if sql[end] == '\'' {
					if end+1 < len(sql) && sql[end+1] == '\'' {
						end += 2
						continue
					}
					inS = false
				}
				end++
				continue
			}
			if sql[end] == '\'' {
				inS = true
				end++
				continue
			}
			if sql[end] == '(' {
				depth++
			}
			if sql[end] == ')' {
				depth--
			}
			end++
		}
		if depth != 0 {
			break
		}
		// Remove from loc[0] (the ||) to end
		sql = sql[:loc[0]] + sql[end:]
	}
	return sql
}

// rewriteArraySubquery converts ARRAY(SELECT ...) to a json_group_array subquery.
// PostgreSQL's ARRAY(SELECT expr FROM ...) constructor is not supported by SQLite.
// We convert it to (SELECT json_group_array(col) FROM (SELECT ...)) which approximates
// the behavior for string concatenation use cases.
func rewriteArraySubquery(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "array(") && !strings.Contains(lower, "array (") {
		return sql
	}
	var result strings.Builder
	i := 0
	n := len(sql)
	inSingle := false
	inDouble := false
	for i < n {
		if inSingle {
			result.WriteByte(sql[i])
			if sql[i] == '\'' {
				if i+1 < n && sql[i+1] == '\'' { result.WriteByte('\''); i += 2; continue }
				inSingle = false
			}
			i++; continue
		}
		if inDouble {
			result.WriteByte(sql[i])
			if sql[i] == '"' {
				if i+1 < n && sql[i+1] == '"' { result.WriteByte('"'); i += 2; continue }
				inDouble = false
			}
			i++; continue
		}
		if sql[i] == '\'' { inSingle = true; result.WriteByte(sql[i]); i++; continue }
		if sql[i] == '"' { inDouble = true; result.WriteByte(sql[i]); i++; continue }

		// Check for ARRAY( pattern (case-insensitive)
		if i+5 < n && strings.EqualFold(sql[i:i+5], "array") {
			// Skip whitespace after ARRAY
			j := i + 5
			for j < n && (sql[j] == ' ' || sql[j] == '\t') { j++ }
			if j < n && sql[j] == '(' {
				// Check if the content starts with SELECT (possibly whitespace)
				k := j + 1
				for k < n && (sql[k] == ' ' || sql[k] == '\t' || sql[k] == '\n') { k++ }
				if k+6 <= n && strings.EqualFold(sql[k:k+6], "select") {
					// Find the matching close paren
					depth := 1
					end := j + 1
					inS := false
					inD := false
					for end < n && depth > 0 {
						if inS {
							if sql[end] == '\'' {
								if end+1 < n && sql[end+1] == '\'' { end += 2; continue }
								inS = false
							}
							end++; continue
						}
						if inD {
							if sql[end] == '"' {
								if end+1 < n && sql[end+1] == '"' { end += 2; continue }
								inD = false
							}
							end++; continue
						}
						if sql[end] == '\'' { inS = true; end++; continue }
						if sql[end] == '"' { inD = true; end++; continue }
						if sql[end] == '(' { depth++ }
						if sql[end] == ')' { depth-- }
						end++
					}
					if depth == 0 {
						// Found matching paren: sql[j+1:end-1] is the SELECT statement
						inner := sql[j+1 : end-1]
						// Wrap in json_group_array subquery
						result.WriteString("(SELECT json_group_array(_arr_val) FROM (")
						result.WriteString(inner)
						result.WriteString(") AS _arr_sub(_arr_val))")
						i = end
						continue
					}
				}
			}
		}
		result.WriteByte(sql[i])
		i++
	}
	return result.String()
}

// rewriteCreateTableWithNoData rewrites CREATE TABLE ... AS SELECT ... WITH NO DATA
// to CREATE TABLE ... AS SELECT ... LIMIT 0.
func rewriteCreateTableWithNoData(sql string) string {
	if !createTableWithNoDataRE.MatchString(sql) {
		return sql
	}
	return createTableWithNoDataRE.ReplaceAllString(sql, "LIMIT 0")
}

// rewriteCreateTableAsParenSelect strips the outer parens from CREATE TABLE ... AS (SELECT ...).
// SQLite requires CREATE TABLE ... AS SELECT ... (no wrapping parens).
var createTableAsParenSelectRE = regexp.MustCompile(`(?i)\bAS\s*\(\s*(SELECT\b)`)

func rewriteCreateTableAsParenSelect(sql string) string {
	lower := strings.ToLower(strings.TrimSpace(sql))
	if !strings.HasPrefix(lower, "create ") && !strings.HasPrefix(lower, "create\t") {
		return sql
	}
	if !strings.Contains(lower, " as ") && !strings.Contains(lower, " as\t") && !strings.Contains(lower, " as\n") {
		return sql
	}
	// Find "AS (SELECT" pattern
	m := createTableAsParenSelectRE.FindStringIndex(sql)
	if m == nil {
		return sql
	}
	asPos := m[0]
	// Find the matching ')' for the '(' that comes right after AS
	openPos := strings.Index(sql[asPos:], "(")
	if openPos < 0 {
		return sql
	}
	openPos += asPos
	closePos := findMatchingCloseParen(sql, openPos)
	if closePos < 0 {
		return sql
	}
	// Only remove the parens if closePos is at the end of the statement (possibly before ';')
	tail := strings.TrimSpace(sql[closePos+1:])
	if tail != "" && tail != ";" {
		return sql
	}
	// Remove open paren and close paren
	inner := sql[openPos+1 : closePos]
	result := sql[:openPos] + inner + sql[closePos+1:]
	return result
}

// findMatchingCloseParen finds the ')' that matches the '(' at openPos.
func findMatchingCloseParen(sql string, openPos int) int {
	depth := 0
	inSingle := false
	inDouble := false
	for i := openPos; i < len(sql); i++ {
		if inSingle {
			if sql[i] == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' { i++; continue }
				inSingle = false
			}
			continue
		}
		if inDouble {
			if sql[i] == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' { i++; continue }
				inDouble = false
			}
			continue
		}
		if sql[i] == '\'' { inSingle = true; continue }
		if sql[i] == '"' { inDouble = true; continue }
		if sql[i] == '(' { depth++ }
		if sql[i] == ')' {
			depth--
			if depth == 0 { return i }
		}
	}
	return -1
}

// rewriteCreateTableColList strips the column name list from
// CREATE TABLE name (col1, col2) AS SELECT ... since SQLite does not support it.
func rewriteCreateTableColList(sql string) string {
	return createTableColListRE.ReplaceAllStringFunc(sql, func(match string) string {
		sub := createTableColListRE.FindStringSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		// sub[1] = "CREATE TABLE ...", sub[2] = col list (may be column names only), sub[3] = "AS "
		// Only strip if it looks like a column name list (no types)
		colList := strings.TrimSpace(sub[2])
		// Heuristic: if the list contains type keywords, it's not just names
		if regexp.MustCompile(`(?i)\b(int|text|varchar|char|bool|float|numeric|date|time)\b`).MatchString(colList) {
			return match
		}
		return sub[1] + " " + sub[3]
	})
}

// rewriteCreateTableColListAsValues handles:
//   CREATE TABLE name (col1, col2, ...) AS VALUES (...)
// SQLite supports CREATE TABLE AS VALUES but gives auto-names (column1, column2, ...).
// We rewrite to: CREATE TABLE name AS VALUES ...; ALTER TABLE name RENAME COLUMN column1 TO col1; ...
var createTableColListAsValuesRE = regexp.MustCompile(
	`(?is)^\s*(CREATE\s+(?:UNLOGGED\s+|TEMP(?:ORARY)?\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z0-9_"]+))\s*\(([^)]+)\)\s*AS\s+(VALUES\s*\([\s\S]*)$`)

func rewriteCreateTableColListAsValues(sql string) string {
	sub := createTableColListAsValuesRE.FindStringSubmatch(sql)
	if len(sub) < 5 {
		return sql
	}
	tablePrefix := sub[1] // "CREATE TABLE name"
	tableName := sub[2]   // "name"
	colListStr := sub[3]  // "col1, col2, ..."
	valuesBody := sub[4]  // "VALUES (...)"

	// Only handle if all columns are plain names (no types)
	if regexp.MustCompile(`(?i)\b(int|text|varchar|char|bool|float|numeric|date|time)\b`).MatchString(colListStr) {
		return sql
	}

	cols := strings.Split(colListStr, ",")
	var colNames []string
	for _, c := range cols {
		name := strings.TrimSpace(c)
		name = strings.Trim(name, `"`)
		if name != "" {
			colNames = append(colNames, name)
		}
	}

	if len(colNames) == 0 {
		return sql
	}

	stmts := []string{tablePrefix + " AS " + valuesBody}
	for i, name := range colNames {
		oldName := fmt.Sprintf("column%d", i+1)
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", tableName, oldName, name))
	}
	return strings.Join(stmts, "; ")
}

// rewriteCreateTableDefaultExpr strips non-constant DEFAULT expressions from
// CREATE TABLE column definitions. SQLite only allows constant literals in DEFAULT.
// e.g. DEFAULT LENGTH(foo(6)) → (removed)
var createTableDefaultFuncRE = regexp.MustCompile(
	`(?i)\bDEFAULT\s+(?:[A-Za-z_][A-Za-z0-9_]*\s*\()`)

func rewriteCreateTableDefaultExpr(sql string) string {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	if !strings.HasPrefix(upper, "CREATE") || !strings.Contains(upper, "TABLE") {
		return sql
	}
	if !strings.Contains(upper, "DEFAULT") {
		return sql
	}
	// Find the opening paren of the column list
	parenStart := strings.Index(sql, "(")
	if parenStart < 0 {
		return sql
	}
	tablePrefix := sql[:parenStart+1]
	// Find the matching closing paren
	depth := 1
	i := parenStart + 1
	for i < len(sql) && depth > 0 {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '\'':
			i++
			for i < len(sql) && sql[i] != '\'' {
				if sql[i] == '\\' {
					i++
				}
				i++
			}
		}
		i++
	}
	colDefs := sql[parenStart+1 : i-1]
	suffix := sql[i-1:]

	// Strip non-constant DEFAULT in column definitions.
	// We need to be careful not to strip DEFAULT inside string literals.
	stripped := stripNonConstantDefaults(colDefs)
	return tablePrefix + stripped + suffix
}

// stripNonConstantDefaults removes DEFAULT <non-constant> from column definitions.
func stripNonConstantDefaults(colDefs string) string {
	// Iterate through colDefs looking for DEFAULT keyword outside of quotes
	var result strings.Builder
	i := 0
	for i < len(colDefs) {
		// Check for string literal
		if colDefs[i] == '\'' {
			result.WriteByte(colDefs[i])
			i++
			for i < len(colDefs) {
				result.WriteByte(colDefs[i])
				if colDefs[i] == '\'' {
					i++
					break
				}
				i++
			}
			continue
		}
		// Check for DEFAULT keyword
		upper := strings.ToUpper(colDefs[i:])
		if strings.HasPrefix(upper, "DEFAULT") && (i == 0 || colDefs[i-1] == ' ' || colDefs[i-1] == '\t' || colDefs[i-1] == '\n') {
			// Read the DEFAULT value
			defStart := i + len("DEFAULT")
			j := defStart
			for j < len(colDefs) && (colDefs[j] == ' ' || colDefs[j] == '\t' || colDefs[j] == '\n') {
				j++
			}
			// Read the default value expression
			defValStart := j
			// Check if it's a function call or complex expr (has '(')
			// vs simple literal
			val, end := readDefaultValue(colDefs, j)
			if isSimpleLiteralDefault(val) || isSupportedDefaultExpr(val) {
				// Keep it
				result.WriteString(colDefs[i:end])
				i = end
			} else {
				// Strip DEFAULT ... entirely
				_ = defValStart
				i = end
			}
			continue
		}
		result.WriteByte(colDefs[i])
		i++
	}
	return result.String()
}

// readAlterColumnDefaultValue reads the DEFAULT value in an ALTER TABLE ADD COLUMN context,
// stopping at SQL constraint keywords (CHECK, NOT, CONSTRAINT, UNIQUE, REFERENCES) or end of string.
func readAlterColumnDefaultValue(s string, j int) (string, int) {
	start := j
	depth := 0
	inStr := false
	for j < len(s) {
		ch := s[j]
		if inStr {
			if ch == '\'' {
				inStr = false
			}
			j++
			continue
		}
		if ch == '\'' {
			inStr = true
			j++
			continue
		}
		if ch == '(' {
			depth++
			j++
			continue
		}
		if ch == ')' {
			if depth == 0 {
				break
			}
			depth--
			j++
			continue
		}
		if depth == 0 {
			if ch == ',' || ch == ';' {
				break
			}
			// Stop at constraint keywords that follow the DEFAULT value
			rest := strings.ToUpper(s[j:])
			for _, kw := range []string{"CHECK ", "CHECK(", "NOT NULL", "CONSTRAINT ", "UNIQUE", "REFERENCES ", "PRIMARY KEY", "DEFERRABLE"} {
				if strings.HasPrefix(rest, kw) {
					return strings.TrimSpace(s[start:j]), j
				}
			}
		}
		j++
	}
	return strings.TrimSpace(s[start:j]), j
}

// readDefaultValue reads the default value starting at position j in s,
// returns the value string and the end position.
func readDefaultValue(s string, j int) (string, int) {
	start := j
	depth := 0
	inStr := false
	for j < len(s) {
		ch := s[j]
		if inStr {
			if ch == '\'' {
				inStr = false
			}
			j++
			continue
		}
		if ch == '\'' {
			inStr = true
			j++
			continue
		}
		if ch == '(' {
			depth++
			j++
			continue
		}
		if ch == ')' {
			if depth == 0 {
				break
			}
			depth--
			j++
			continue
		}
		if depth == 0 && ch == ',' {
			break
		}
		j++
	}
	return strings.TrimSpace(s[start:j]), j
}

// isSupportedDefaultExpr returns true if the default expression uses known
// SQLite/minipsql functions that are registered and safe in DEFAULT contexts.
func isSupportedDefaultExpr(val string) bool {
	val = strings.TrimSpace(val)
	upper := strings.ToUpper(val)
	// nextval, currval, gen_random_uuid are registered minipsql functions
	for _, fn := range []string{"NEXTVAL(", "CURRVAL(", "GEN_RANDOM_UUID(", "RANDOM(", "ABS("} {
		if strings.HasPrefix(upper, fn) {
			return true
		}
	}
	// (nextval(...)) — wrapped in parens
	if strings.HasPrefix(val, "(") {
		inner := strings.TrimSpace(val[1:])
		end := strings.LastIndex(inner, ")")
		if end >= 0 {
			return isSupportedDefaultExpr(inner[:end])
		}
	}
	return false
}

// rewriteAlterAddColumnDefaultExpr strips non-constant DEFAULT expressions from
// ALTER TABLE ... ADD COLUMN col TYPE DEFAULT expr, since SQLite cannot add a column
// with a non-constant default to a table that already has rows.
// Simple literals (integers, quoted strings, NULL, TRUE, FALSE) are kept.
var alterAddColDefaultRE = regexp.MustCompile(
	`(?i)(\bADD\s+(?:COLUMN\s+)?"?[A-Za-z0-9_]+"?\s+[A-Za-z0-9_(, ]+?)\s+DEFAULT\s+(.+)$`)

func rewriteAlterAddColumnDefaultExpr(sql string) string {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	if !strings.HasPrefix(upper, "ALTER TABLE") {
		return sql
	}
	if !strings.Contains(upper, "DEFAULT") {
		return sql
	}
	// Only handle single ADD COLUMN (multi-action was already split above)
	if !strings.Contains(upper, "ADD ") {
		return sql
	}
	// Skip if this is a multi-statement (semicolon present) — handled per-statement by Execute
	if strings.Contains(sql, ";") {
		return sql
	}

	// Find "DEFAULT" after ADD [COLUMN]
	// We need to strip DEFAULT if the expression is not a simple literal.
	// Walk through to find "ADD [COLUMN]" then find "DEFAULT"
	lower := strings.ToLower(sql)
	addIdx := strings.Index(lower, " add ")
	if addIdx < 0 {
		return sql
	}
	// Find DEFAULT after ADD
	defIdx := strings.Index(lower[addIdx:], " default ")
	if defIdx < 0 {
		return sql
	}
	defIdx += addIdx // absolute position of " default "
	defValStart := defIdx + len(" default ")
	// Read the default value, stopping at constraint keywords (CHECK, NOT, NULL, UNIQUE, REFERENCES)
	defVal, _ := readAlterColumnDefaultValue(sql, defValStart)

	// Check if defVal is a simple constant
	if isSimpleLiteralDefault(defVal) {
		return sql
	}
	// Strip the DEFAULT clause (but keep any trailing constraint keywords)
	suffix := strings.TrimSpace(sql[defValStart+len(defVal):])
	if suffix != "" {
		return strings.TrimRight(sql[:defIdx], " \t") + " " + suffix
	}
	return strings.TrimRight(sql[:defIdx], " \t")
}

// isSimpleLiteralDefault returns true if the default value is a simple constant
// that SQLite can handle in ADD COLUMN DEFAULT.
func isSimpleLiteralDefault(val string) bool {
	val = strings.TrimSpace(val)
	// Strip trailing semicolons
	val = strings.TrimRight(val, "; \t")
	upper := strings.ToUpper(val)
	if upper == "NULL" || upper == "TRUE" || upper == "FALSE" {
		return true
	}
	// Number literal (possibly negative)
	numRE := regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	if numRE.MatchString(val) {
		return true
	}
	// Simple quoted string
	if (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) ||
		(strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) {
		return true
	}
	return false
}

// rewriteWithOrdinality strips WITH ORDINALITY (not supported by SQLite).
func rewriteWithOrdinality(sql string) string {
	return withOrdinalityRE.ReplaceAllString(sql, "")
}

// rewritePGCatalogOperator rewrites OPERATOR(pg_catalog.op) to the bare operator for SQLite compatibility.
// e.g. OPERATOR(pg_catalog.~) -> REGEXP, OPERATOR(pg_catalog.=) -> =
func rewritePGCatalogOperator(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "operator") {
		return sql
	}
	return pgCatalogOperatorRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := pgCatalogOperatorRE.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		op := sub[1]
		switch op {
		case "~":
			return "REGEXP"
		case "~*":
			return "REGEXP" // case-insensitive, approximate
		case "=":
			return "="
		case "<>", "!=":
			return "!="
		case "<":
			return "<"
		case ">":
			return ">"
		case "<=":
			return "<="
		case ">=":
			return ">="
		case "||":
			return "||"
		default:
			return op
		}
	})
}

// rewriteGeneratedColumns converts PostgreSQL generated column syntax to SQLite-compatible form.
// - GENERATED ALWAYS|BY DEFAULT AS IDENTITY → stripped (no identity columns in SQLite)
// - GENERATED ALWAYS AS (expr) STORED/VIRTUAL → GENERATED ALWAYS AS (expr) (SQLite supports virtual only)
// - GENERATED BY DEFAULT AS (expr) STORED → GENERATED ALWAYS AS (expr)
func rewriteGeneratedColumns(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "generated") {
		return sql
	}
	// Strip IDENTITY columns: "GENERATED ALWAYS AS IDENTITY" → ""
	out := generatedIdentityRE.ReplaceAllString(sql, "")
	// Rewrite "GENERATED ALWAYS AS (expr) STORED" → "GENERATED ALWAYS AS (expr)"
	out = generatedStoredRE.ReplaceAllString(out, " GENERATED ALWAYS AS ($1)")
	// Rewrite "GENERATED BY DEFAULT AS (expr) STORED" → "GENERATED ALWAYS AS (expr)"
	out = generatedByDefaultStoredRE.ReplaceAllString(out, " GENERATED ALWAYS AS ($1)")
	return out
}

// rewriteSubqueryColumnAlias rewrites subquery column aliases:
//   (SELECT e1, e2 FROM ...) AS alias(col1, col2)
//   → (SELECT e1 AS col1, e2 AS col2 FROM ...) AS alias
// SQLite does not support column name lists after subquery aliases.
// Also handles VALUES column aliases:
//   (VALUES ...) AS alias(col1, col2) → (SELECT * FROM (VALUES ...) AS alias) AS alias
func rewriteSubqueryColumnAlias(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, ") as ") && !strings.Contains(lower, ") as\t") &&
		!strings.Contains(lower, ") as\n") {
		return sql
	}

	// Find all ") AS alias(cols)" patterns and collect replacements
	type replacement struct {
		start    int // start of the subquery opening '('
		closePos int // position of the subquery closing ')'
		colEnd   int // position after the col alias list ')'
		aliases  []string
		alias    string
	}
	var replacements []replacement

	n := len(sql)
	i := 0
	// Track paren depths to find matching open parens
	// We scan for ") AS alias(col_list)" and then backtrack to find matching '('
	for i < n {
		// Skip string literals
		if sql[i] == '\'' {
			i++
			for i < n {
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' { i += 2; continue }
					break
				}
				i++
			}
			i++
			continue
		}
		if sql[i] == '"' {
			i++
			for i < n {
				if sql[i] == '"' {
					if i+1 < n && sql[i+1] == '"' { i += 2; continue }
					break
				}
				i++
			}
			i++
			continue
		}
		// Dollar-quoted strings
		if sql[i] == '$' {
			j := i + 1
			for j < n && sql[j] != '$' && (isIdentByte(sql[j]) || sql[j] == '_') { j++ }
			if j < n && sql[j] == '$' {
				tag := sql[i : j+1]
				k := j + 1
				for k < n {
					idx := strings.Index(sql[k:], tag)
					if idx < 0 { break }
					k = k + idx + len(tag)
					break
				}
				i = k
				continue
			}
		}

		if sql[i] != ')' { i++; continue }

		closePos := i
		// skip whitespace after ')'
		k := closePos + 1
		for k < n && (sql[k] == ' ' || sql[k] == '\t' || sql[k] == '\n' || sql[k] == '\r') { k++ }
		// check for AS keyword
		if k+2 > n || !strings.EqualFold(sql[k:k+2], "as") { i++; continue }
		if k+2 < n && isIdentByte(sql[k+2]) { i++; continue }
		k += 2
		for k < n && (sql[k] == ' ' || sql[k] == '\t' || sql[k] == '\n' || sql[k] == '\r') { k++ }
		// read alias identifier
		aliasStart := k
		if k < n && sql[k] == '"' {
			k++
			for k < n && sql[k] != '"' { k++ }
			if k < n { k++ }
		} else {
			for k < n && isIdentByte(sql[k]) { k++ }
		}
		if k <= aliasStart { i++; continue }
		aliasName := sql[aliasStart:k]
		// skip whitespace
		for k < n && (sql[k] == ' ' || sql[k] == '\t') { k++ }
		// check for opening paren (column alias list)
		if k >= n || sql[k] != '(' { i++; continue }
		// parse column alias list: only identifiers and commas
		closeP := k + 1
		valid := true
		for closeP < n && sql[closeP] != ')' {
			if sql[closeP] == '(' { valid = false; break }
			if sql[closeP] == '\'' || sql[closeP] == '"' { valid = false; break }
			closeP++
		}
		if !valid || closeP >= n { i++; continue }
		// Extract column aliases
		colList := strings.TrimSpace(sql[k+1 : closeP])
		if colList == "" { i++; continue }
		colAliases := strings.Split(colList, ",")
		for ci := range colAliases {
			colAliases[ci] = strings.TrimSpace(colAliases[ci])
		}
		// Find matching opening '(' for the subquery by scanning backwards
		openPos := findMatchingOpenParen(sql, closePos)
		if openPos < 0 { i++; continue }
		replacements = append(replacements, replacement{
			start:   openPos,
			closePos: closePos,
			colEnd:  closeP + 1,
			aliases: colAliases,
			alias:   aliasName,
		})
		i = closeP + 1
	}

	if len(replacements) == 0 {
		return sql
	}

	// Apply replacements from right to left to preserve positions
	for ri := len(replacements) - 1; ri >= 0; ri-- {
		r := replacements[ri]
		subquery := sql[r.start+1 : r.closePos] // content between outer parens
		rewritten := rewriteSelectAliases(subquery, r.aliases)
		sql = sql[:r.start] + "(" + rewritten + ") AS " + r.alias + sql[r.colEnd:]
	}
	return sql
}

// findMatchingOpenParen finds the '(' that matches the ')' at closePos.
func findMatchingOpenParen(sql string, closePos int) int {
	depth := 0
	for i := closePos; i >= 0; i-- {
		if sql[i] == ')' { depth++ }
		if sql[i] == '(' {
			depth--
			if depth == 0 { return i }
		}
		// Skip string literals going backwards is complex; do a simplified version
		// that handles most cases (no embedded strings with parens)
	}
	return -1
}

// rewriteSelectAliases adds AS aliases to the SELECT list of a subquery body.
// aliases is the list of column names to assign to each SELECT expression.
func rewriteSelectAliases(body string, aliases []string) string {
	if len(aliases) == 0 { return body }
	lower := strings.ToLower(strings.TrimSpace(body))
	if !strings.HasPrefix(lower, "select ") && !strings.HasPrefix(lower, "select\t") &&
		!strings.HasPrefix(lower, "select\n") {
		return body
	}
	// Find the SELECT keyword and then the FROM keyword at top level
	// Parse the SELECT list by splitting on commas at paren depth 0
	// Find "select" position
	selectEnd := 7 // len("select ") or "select\t" etc.
	// Skip DISTINCT / ALL / DISTINCT ON (...)
	trimmed := strings.TrimSpace(body[selectEnd:])
	ltrimmed := strings.ToLower(trimmed)
	prefixLen := 0
	if strings.HasPrefix(ltrimmed, "distinct on (") {
		depth := 0
		for pi, ch := range ltrimmed {
			if ch == '(' { depth++ }
			if ch == ')' { depth--; if depth == 0 { prefixLen = pi + 1; break } }
		}
	} else if strings.HasPrefix(ltrimmed, "distinct ") {
		prefixLen = 9
	} else if strings.HasPrefix(ltrimmed, "all ") {
		prefixLen = 4
	}
	selectListStart := selectEnd + (len(trimmed) - len(strings.TrimLeft(trimmed, ""))) + prefixLen
	// Recompute properly
	afterSelect := body[selectEnd:]
	wsLen := len(afterSelect) - len(strings.TrimLeft(afterSelect, " \t\n\r"))
	afterSelect = afterSelect[wsLen:]
	lalter := strings.ToLower(afterSelect)
	skip := 0
	if strings.HasPrefix(lalter, "distinct on (") {
		depth := 0
		for pi := 0; pi < len(lalter); pi++ {
			if lalter[pi] == '(' { depth++ }
			if lalter[pi] == ')' { depth--; if depth == 0 { skip = pi + 1; break } }
		}
		skip += len(strings.TrimLeft(lalter[skip:], " \t\n\r")) - len(lalter[skip:])
		// advance past whitespace
		for skip < len(afterSelect) && (afterSelect[skip] == ' ' || afterSelect[skip] == '\t') { skip++ }
	} else if strings.HasPrefix(lalter, "distinct ") {
		skip = 9
	} else if strings.HasPrefix(lalter, "all ") {
		skip = 4
	}
	selectListStart = selectEnd + wsLen + skip
	_ = selectListStart

	// Find the SELECT list: everything between after DISTINCT/ALL and FROM at depth 0
	selectContent := afterSelect[skip:]
	// Find the position of the select list and the FROM clause
	fromIdx := findFromAtTopLevel(selectContent)
	var selectList string
	var rest string
	if fromIdx < 0 {
		selectList = selectContent
		rest = ""
	} else {
		selectList = selectContent[:fromIdx]
		rest = selectContent[fromIdx:]
	}

	// Split select list by commas at depth 0
	exprs := splitTopLevelCommas(selectList)
	if len(exprs) != len(aliases) {
		// Can't safely add aliases if counts don't match — fall back to stripping
		return body
	}

	// Add aliases to each expression
	var newExprs []string
	for idx, expr := range exprs {
		expr = strings.TrimSpace(expr)
		elower := strings.ToLower(expr)
		// Check if already has an alias
		// Simple check: if the expr ends with "as name" pattern
		if hasAlias(elower) {
			newExprs = append(newExprs, expr)
		} else {
			newExprs = append(newExprs, expr+" AS "+aliases[idx])
		}
	}

	prefix := body[:selectEnd+wsLen]
	if skip > 0 {
		prefix += afterSelect[:skip]
	}
	sep := ""
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' && rest[0] != '\n' && rest[0] != '\r' {
		sep = " "
	}
	return prefix + strings.Join(newExprs, ", ") + sep + rest
}

// findFromAtTopLevel finds the position of the FROM keyword at top-level (paren depth 0).
func findFromAtTopLevel(s string) int {
	lower := strings.ToLower(s)
	n := len(s)
	depth := 0
	inSingle := false
	for i := 0; i < n; i++ {
		if inSingle {
			if s[i] == '\'' {
				if i+1 < n && s[i+1] == '\'' { i++; continue }
				inSingle = false
			}
			continue
		}
		if s[i] == '\'' { inSingle = true; continue }
		if s[i] == '(' { depth++; continue }
		if s[i] == ')' { depth--; continue }
		if depth == 0 && i+4 <= n && strings.EqualFold(lower[i:i+4], "from") {
			// must be followed by whitespace or end
			if i+4 >= n || lower[i+4] == ' ' || lower[i+4] == '\t' || lower[i+4] == '\n' || lower[i+4] == '\r' {
				// must be preceded by whitespace or start
				if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' || s[i-1] == '\n' || s[i-1] == '\r' {
					return i
				}
			}
		}
	}
	return -1
}

// splitTopLevelCommas splits a SQL expression list by commas at paren depth 0.
func splitTopLevelCommas(s string) []string {
	var result []string
	depth := 0
	inSingle := false
	inDouble := false
	start := 0
	n := len(s)
	for i := 0; i < n; i++ {
		if inSingle {
			if s[i] == '\'' {
				if i+1 < n && s[i+1] == '\'' { i++; continue }
				inSingle = false
			}
			continue
		}
		if inDouble {
			if s[i] == '"' {
				if i+1 < n && s[i+1] == '"' { i++; continue }
				inDouble = false
			}
			continue
		}
		if s[i] == '\'' { inSingle = true; continue }
		if s[i] == '"' { inDouble = true; continue }
		if s[i] == '(' { depth++; continue }
		if s[i] == ')' { depth--; continue }
		if depth == 0 && s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// hasAlias returns true if a SELECT expression already ends with "AS name".
func hasAlias(lower string) bool {
	lower = strings.TrimSpace(lower)
	// Look for " as " followed by an identifier at the end
	idx := strings.LastIndex(lower, " as ")
	if idx < 0 { return false }
	rest := strings.TrimSpace(lower[idx+4:])
	// rest should be a simple identifier (no spaces, no parens)
	if rest == "" || strings.ContainsAny(rest, " \t\n(),'\"") { return false }
	return true
}

// rewriteDropCascade strips CASCADE/RESTRICT from DROP statements that SQLite doesn't support.
func rewriteDropCascade(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "cascade") && !strings.Contains(lower, "restrict") {
		return sql
	}
	return dropCascadeRE.ReplaceAllString(sql, "$1$3")
}

// rewritePartitionBy strips PARTITION BY clauses from CREATE TABLE (SQLite has no partitioning).
// CREATE TABLE ... PARTITION OF ... (partition child tables) become regular CREATE TABLE.
func rewritePartitionBy(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "partition") {
		return sql
	}
	out := stripPartitionByClause(sql)
	// Rewrite "CREATE TABLE t PARTITION OF parent FOR VALUES ..." to "CREATE TABLE IF NOT EXISTS t (LIKE parent)"
	if partitionOfRE.MatchString(out) {
		m := regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+((?:IF\s+NOT\s+EXISTS\s+)?[A-Za-z0-9_".]+)\s+PARTITION\s+OF\s+([A-Za-z0-9_".]+)`).FindStringSubmatch(out)
		if m != nil {
			return "CREATE TABLE IF NOT EXISTS " + m[1] + " (LIKE " + m[2] + ")"
		}
	}
	return out
}

// stripPartitionByClause removes PARTITION BY RANGE/LIST/HASH (...) handling nested parens.
func stripPartitionByClause(sql string) string {
	result := sql
	for {
		loc := partitionByClauseRE.FindStringIndex(result)
		if loc == nil {
			break
		}
		// loc[1] points to the char after the opening '('
		depth := 1
		i := loc[1]
		for i < len(result) && depth > 0 {
			switch result[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			i++
		}
		result = result[:loc[0]] + result[i:]
	}
	return result
}

// rewriteOffsetBeforeLimit swaps "OFFSET m LIMIT n" to "LIMIT n OFFSET m" for SQLite compatibility.
func rewriteOffsetBeforeLimit(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "offset") || !strings.Contains(lower, "limit") {
		return sql
	}
	return offsetBeforeLimitRE.ReplaceAllString(sql, "LIMIT $2 OFFSET $1")
}

// rewriteBareOffset rewrites "OFFSET n" without a preceding LIMIT clause to "LIMIT -1 OFFSET n".
// SQLite requires LIMIT when OFFSET is used.
// Handles both standalone and subquery OFFSET.
func rewriteBareOffset(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "offset") {
		return sql
	}
	// Look for each OFFSET occurrence; check if there's a LIMIT in the same scope
	var result strings.Builder
	i := 0
	n := len(sql)
	inSingle := false
	inDouble := false
	for i < n {
		if inSingle {
			if sql[i] == '\'' && i+1 < n && sql[i+1] == '\'' {
				result.WriteString("''")
				i += 2
				continue
			}
			if sql[i] == '\'' {
				inSingle = false
			}
			result.WriteByte(sql[i])
			i++
			continue
		}
		if inDouble {
			if sql[i] == '"' && i+1 < n && sql[i+1] == '"' {
				result.WriteString(`""`)
				i += 2
				continue
			}
			if sql[i] == '"' {
				inDouble = false
			}
			result.WriteByte(sql[i])
			i++
			continue
		}
		if sql[i] == '\'' {
			inSingle = true
			result.WriteByte(sql[i])
			i++
			continue
		}
		if sql[i] == '"' {
			inDouble = true
			result.WriteByte(sql[i])
			i++
			continue
		}
		// Check for OFFSET keyword
		if i+6 <= n && strings.ToLower(sql[i:i+6]) == "offset" {
			leftBound := i == 0 || !isIdentByte(sql[i-1])
			rightBound := i+6 == n || !isIdentByte(sql[i+6])
			if leftBound && rightBound {
				// Extract the number after OFFSET
				j := i + 6
				for j < n && (sql[j] == ' ' || sql[j] == '\t') {
					j++
				}
				numStart := j
				isParenExpr := j < n && sql[j] == '('
				for j < n && sql[j] >= '0' && sql[j] <= '9' {
					j++
				}
				if j > numStart || isParenExpr {
					var numStr string
					k := j
					if isParenExpr {
						// Read balanced parenthesized expression for OFFSET
						depth2 := 1
						k = numStart + 1
						for k < n && depth2 > 0 {
							if sql[k] == '(' { depth2++ }
							if sql[k] == ')' { depth2-- }
							k++
						}
						numStr = sql[numStart:k]
					} else {
						numStr = sql[numStart:j]
						// Skip optional ROWS/ROW
						for k < n && (sql[k] == ' ' || sql[k] == '\t') {
							k++
						}
						if k+4 <= n && strings.ToLower(sql[k:k+4]) == "rows" {
							k += 4
						} else if k+3 <= n && strings.ToLower(sql[k:k+3]) == "row" {
							k += 3
						} else {
							k = j
						}
					}
					// Check if LIMIT exists before OFFSET in same "scope" (search backwards)
					scopeStart := findScopeStart(sql, i)
					scopeText := strings.ToLower(sql[scopeStart:i])
					hasFetch := strings.Contains(scopeText, "fetch")
					hasLimit := strings.Contains(scopeText, "limit")
					if !hasLimit && !hasFetch {
						result.WriteString("LIMIT -1 OFFSET ")
						result.WriteString(numStr)
						i = k
						continue
					}
				}
			}
		}
		result.WriteByte(sql[i])
		i++
	}
	return result.String()
}

// findScopeStart finds where the current SELECT scope starts (at a '(' or beginning).
func findScopeStart(sql string, pos int) int {
	depth := 0
	i := pos - 1
	for i >= 0 {
		switch sql[i] {
		case ')':
			depth++
		case '(':
			if depth == 0 {
				return i + 1
			}
			depth--
		}
		i--
	}
	return 0
}

// rewriteCreateIndex adds IF NOT EXISTS to named CREATE INDEX to prevent duplicate errors.
func rewriteCreateIndex(sql string) string {
	return createIndexRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := createIndexRE.FindStringSubmatch(m)
		if sub[2] != "" {
			return m // already has IF NOT EXISTS
		}
		// skip anonymous index: name would be "ON" (CREATE INDEX ON table)
		if strings.EqualFold(sub[3], "ON") {
			return m
		}
		return sub[1] + "IF NOT EXISTS " + sub[3]
	})
}

// rewriteCollateClause maps PostgreSQL collation names to SQLite equivalents.
// SQLite supports BINARY, NOCASE, RTRIM. Unknown collations are stripped.
func rewriteCollateClause(sql string) string {
	if !strings.Contains(strings.ToLower(sql), "collate") {
		return sql
	}
	return collateClauseRE.ReplaceAllStringFunc(sql, func(m string) string {
		sub := collateClauseRE.FindStringSubmatch(m)
		name := sub[1]
		if name == "" {
			name = sub[2]
		}
		nameLower := strings.ToLower(name)
		switch {
		case nameLower == "c" || nameLower == "posix" || nameLower == "binary":
			return "COLLATE BINARY"
		case strings.HasPrefix(nameLower, "en") || strings.HasPrefix(nameLower, "nocase"):
			return "COLLATE NOCASE"
		default:
			return "" // strip unknown collations
		}
	})
}

// rewriteIndexInclude strips INCLUDE (col1, col2) from CREATE INDEX statements.
// SQLite does not support covering index INCLUDE columns.
func rewriteIndexInclude(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "include") {
		return sql
	}
	if !strings.Contains(lower, "index") {
		return sql
	}
	return indexIncludeRE.ReplaceAllString(sql, "")
}

// rewriteColumnCompression strips COMPRESSION method from column definitions.
func rewriteColumnCompression(sql string) string {
	return compressionRE.ReplaceAllString(sql, "")
}

// rewriteInsertSelect rewrites INSERT INTO t (SELECT ...) to INSERT INTO t SELECT ...
// SQLite requires SELECT without wrapping parens in INSERT.
func rewriteInsertSelect(sql string) string {
	return insertSelectRE.ReplaceAllString(sql, "$1 $2")
}

// rewriteInsertDefault replaces DEFAULT in INSERT VALUES lists with NULL.
// SQLite does not support DEFAULT in VALUES lists (only DEFAULT VALUES).
// This is an approximation — columns with non-NULL defaults may get NULL.
func rewriteInsertDefault(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "default") {
		return sql
	}
	if !strings.Contains(lower, "insert") {
		return sql
	}
	// Find VALUES keyword and replace DEFAULT within the values list
	valuesIdx := indexKeywordOutsideQuotesAndParens(lower, "values")
	if valuesIdx == -1 {
		return sql
	}
	// Everything before VALUES stays the same
	prefix := sql[:valuesIdx+len("values")]
	suffix := sql[valuesIdx+len("values"):]
	// In the VALUES section, replace standalone DEFAULT with NULL
	suffixLower := strings.ToLower(suffix)
	var result strings.Builder
	i := 0
	n := len(suffix)
	inSingle := false
	inDouble := false
	for i < n {
		if inSingle {
			if suffix[i] == '\'' && i+1 < n && suffix[i+1] == '\'' {
				result.WriteString("''")
				i += 2
				continue
			}
			if suffix[i] == '\'' {
				inSingle = false
			}
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if inDouble {
			if suffix[i] == '"' && i+1 < n && suffix[i+1] == '"' {
				result.WriteString(`""`)
				i += 2
				continue
			}
			if suffix[i] == '"' {
				inDouble = false
			}
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if suffix[i] == '\'' {
			inSingle = true
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if suffix[i] == '"' {
			inDouble = true
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if i+7 <= n && strings.ToLower(suffixLower[i:i+7]) == "default" {
			leftBound := i == 0 || !isIdentByte(suffix[i-1])
			rightBound := i+7 == n || !isIdentByte(suffix[i+7])
			if leftBound && rightBound {
				result.WriteString("NULL")
				i += 7
				continue
			}
		}
		result.WriteByte(suffix[i])
		i++
	}
	return prefix + result.String()
}

// rewriteWithOids strips WITH OIDS and WITHOUT OIDS clauses (legacy PostgreSQL).
func rewriteWithOids(sql string) string {
	result := withOidsRE.ReplaceAllString(sql, "")
	result = withoutOidsRE.ReplaceAllString(result, "")
	return result
}

// rewriteEmptyTable adds a dummy column to empty table definitions.
// SQLite does not support tables with no columns.
func rewriteEmptyTable(sql string) string {
	return emptyTableRE.ReplaceAllString(sql, "$1 (_dummy integer)")
}

// rewriteForUpdate strips SELECT ... FOR UPDATE/SHARE locking clauses.
// SQLite does not support row-level locking.
func rewriteForUpdate(sql string) string {
	return forUpdateRE.ReplaceAllString(sql, "")
}

// rewriteIndexOpClasses strips PostgreSQL operator class names from CREATE INDEX.
// e.g. "col int4_ops" → "col"
func rewriteIndexOpClasses(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "create") || !strings.Contains(lower, "index") {
		return sql
	}
	return indexOpClassRE.ReplaceAllString(sql, "$1")
}

// rewriteNullsFirstLast strips NULLS FIRST / NULLS LAST from ORDER BY clauses
// where SQLite does not support them.
func rewriteNullsFirstLast(sql string) string {
	return nullsFirstLastRE.ReplaceAllString(sql, "")
}

// rewriteDropMultiTable rewrites DROP TABLE a, b, c to multiple DROP TABLE statements.
// SQLite does not support dropping multiple tables in a single statement.
func rewriteDropMultiTable(sql string) string {
	sub := dropMultiTableRE.FindStringSubmatch(sql)
	if sub == nil {
		return sql
	}
	prefix := sub[1] // "DROP TABLE " or "DROP TABLE IF EXISTS "
	tableList := sub[2]
	// sub[3] is CASCADE/RESTRICT — we ignore it (strip it)

	tables := strings.Split(tableList, ",")
	stmts := make([]string, 0, len(tables))
	for _, t := range tables {
		t = strings.TrimSpace(t)
		if t != "" {
			stmts = append(stmts, prefix+t)
		}
	}
	if len(stmts) == 0 {
		return sql
	}
	return strings.Join(stmts, "; ")
}

// rewritePGNumericOps rewrites PostgreSQL-specific numeric operators:
//   @expr    → ABS(expr)
//   |/ expr  → SQRT(expr)
//   ||/ expr → POWER(expr, 1.0/3.0)  [cubic root]
//   a ^ b    → POWER(a, b)           [exponentiation — PG uses ^ for power, not bitXOR]
func rewritePGNumericOps(sql string) string {
	if !strings.ContainsAny(sql, "@|^#") {
		return sql
	}
	var b strings.Builder
	n := len(sql)
	inSingle := false
	inDouble := false
	i := 0
	for i < n {
		if inSingle {
			b.WriteByte(sql[i])
			if sql[i] == '\'' {
				if i+1 < n && sql[i+1] == '\'' {
					b.WriteByte('\''); i += 2; continue
				}
				inSingle = false
			}
			i++; continue
		}
		if inDouble {
			b.WriteByte(sql[i])
			if sql[i] == '"' {
				if i+1 < n && sql[i+1] == '"' {
					b.WriteByte('"'); i += 2; continue
				}
				inDouble = false
			}
			i++; continue
		}
		if sql[i] == '\'' { inSingle = true; b.WriteByte(sql[i]); i++; continue }
		if sql[i] == '"' { inDouble = true; b.WriteByte(sql[i]); i++; continue }

		// ||/ cubic root (must check before |/)
		if i+3 <= n && sql[i:i+3] == "||/" {
			// check this is an operator context (preceded by space, (, comma, operator, or start)
			if isOpContext(sql, i) {
				j := i + 3
				for j < n && (sql[j] == ' ' || sql[j] == '\t') { j++ }
				expr, end := extractOperand(sql, j)
				if expr != "" {
					b.WriteString("POWER(" + expr + ", 1.0/3.0)")
					i = end; continue
				}
			}
		}
		// |/ square root
		if i+2 <= n && sql[i:i+2] == "|/" && (i+2 >= n || sql[i+2] != '/') {
			if isOpContext(sql, i) {
				j := i + 2
				for j < n && (sql[j] == ' ' || sql[j] == '\t') { j++ }
				expr, end := extractOperand(sql, j)
				if expr != "" {
					b.WriteString("SQRT(" + expr + ")")
					i = end; continue
				}
			}
		}
		// @ absolute value (unary prefix) — avoid matching @... in identifiers
		if sql[i] == '@' && (i == 0 || isOpContext(sql, i)) && i+1 < n && sql[i+1] != '>' && sql[i+1] != '@' {
			// Check not part of an identifier (@ is not valid in SQL identifiers)
			j := i + 1
			for j < n && (sql[j] == ' ' || sql[j] == '\t') { j++ }
			expr, end := extractOperand(sql, j)
			if expr != "" {
				b.WriteString("ABS(" + expr + ")")
				i = end; continue
			}
		}
		// # bitwise XOR (binary operator: left # right → pg_bitxor(left, right))
		// But NOT #> or #- (jsonb path operators)
		if sql[i] == '#' && i+1 < n && sql[i+1] != '>' && sql[i+1] != '-' && sql[i+1] != '#' {
			if i > 0 && (isIdentByte(sql[i-1]) || sql[i-1] == ' ' || sql[i-1] == '\t' || sql[i-1] == ')' || sql[i-1] == '\'') {
				j := i + 1
				for j < n && (sql[j] == ' ' || sql[j] == '\t') { j++ }
				rExpr, rEnd := extractOperand(sql, j)
				if rExpr != "" {
					bStr := b.String()
					lExpr, lStart := extractOperandBackward(bStr)
					if lExpr != "" {
						newB := bStr[:lStart]
						b.Reset()
						b.WriteString(newB)
						b.WriteString("pg_bitxor(" + lExpr + ", " + rExpr + ")")
						i = rEnd; continue
					}
				}
			}
		}
		// ^ exponentiation (binary operator: left ^ right → POWER(left, right))
		// PG uses ^ for power, SQLite uses ^ for bitXOR — always convert.
		if sql[i] == '^' {
			// Ensure this is not inside a string (handled by inSingle/inDouble above)
			// Extract right operand
			j := i + 1
			for j < n && (sql[j] == ' ' || sql[j] == '\t') { j++ }
			rExpr, rEnd := extractOperand(sql, j)
			if rExpr != "" {
				// Extract left operand (scan backwards in b)
				bStr := b.String()
				lExpr, lStart := extractOperandBackward(bStr)
				if lExpr != "" {
					newB := bStr[:lStart]
					b.Reset()
					b.WriteString(newB)
					b.WriteString("POWER(" + lExpr + ", " + rExpr + ")")
					i = rEnd; continue
				}
			}
		}
		b.WriteByte(sql[i])
		i++
	}
	return b.String()
}

// extractOperandBackward extracts the last operand from s, scanning backwards.
// Returns the operand string and its start index in s.
func extractOperandBackward(s string) (string, int) {
	n := len(s)
	if n == 0 { return "", 0 }
	i := n - 1
	// Skip trailing whitespace
	for i >= 0 && (s[i] == ' ' || s[i] == '\t') { i-- }
	if i < 0 { return "", 0 }
	end := i + 1
	// String literal
	if s[i] == '\'' {
		i--
		for i >= 0 {
			if s[i] == '\'' {
				if i > 0 && s[i-1] == '\'' { i -= 2; continue }
				break
			}
			i--
		}
		if i < 0 { return "", 0 }
		return s[i:end], i
	}
	// Closing paren — find matching open
	if s[i] == ')' {
		depth := 1; i--
		for i >= 0 && depth > 0 {
			if s[i] == ')' { depth++ } else if s[i] == '(' { depth-- }
			i--
		}
		i++
		// skip preceding function name
		for i > 0 && isIdentByte(s[i-1]) { i-- }
		return s[i:end], i
	}
	// Identifier, number, or qualified name (a.b.c)
	for i > 0 && (isIdentByte(s[i-1]) || s[i-1] == '.') { i-- }
	if i == end { return "", 0 }
	return s[i:end], i
}

// isOpContext returns true if position i is an operator context
// (preceded by whitespace, open paren, comma, or start of string).
func isOpContext(sql string, i int) bool {
	if i == 0 { return true }
	prev := sql[i-1]
	return prev == ' ' || prev == '\t' || prev == '\n' || prev == '(' || prev == ',' || prev == '=' || prev == '>' || prev == '<'
}

// extractOperand extracts a single SQL operand starting at pos.
// Returns the operand string and the end position.
func extractOperand(sql string, pos int) (string, int) {
	n := len(sql)
	if pos >= n { return "", pos }
	// Function call or parenthesized expr
	if sql[pos] == '(' {
		end := findMatchingParen(sql, pos)
		if end < 0 { return "", pos }
		// check for .field after paren (qualified name)
		return sql[pos : end+1], end + 1
	}
	// String literal
	if sql[pos] == '\'' {
		j := pos + 1
		for j < n {
			if sql[j] == '\'' {
				if j+1 < n && sql[j+1] == '\'' { j += 2; continue }
				j++; break
			}
			j++
		}
		return sql[pos:j], j
	}
	// Identifier or number (possibly qualified: a.b, a.b.c, or function call)
	j := pos
	for j < n && (isIdentByte(sql[j]) || sql[j] == '.' || sql[j] == '-' || sql[j] == '+') {
		j++
	}
	// Function call
	if j < n && sql[j] == '(' {
		end := findMatchingParen(sql, j)
		if end >= 0 { return sql[pos : end+1], end + 1 }
	}
	if j == pos { return "", pos }
	return sql[pos:j], j
}

// rewriteExtract rewrites EXTRACT(field FROM expr) to date_part('field', expr).
// This is needed because SQLite does not support EXTRACT syntax.
func rewriteExtract(sql string) string {
	result := extractRE.ReplaceAllStringFunc(sql, func(match string) string {
		// match is "EXTRACT(field FROM " — we need to find the field name
		subm := extractRE.FindStringSubmatch(match)
		if len(subm) < 2 {
			return match
		}
		field := strings.ToLower(subm[1])
		return fmt.Sprintf("date_part('%s', ", field)
	})
	// The closing paren of EXTRACT is kept as-is since we just replaced the opening
	return result
}

// rewriteStandaloneUser replaces the bare USER keyword with current_user(),
// skipping occurrences that are part of CURRENT_USER or SESSION_USER.
func rewriteStandaloneUser(sql string) string {
	return bareUserRE.ReplaceAllStringFunc(sql, func(m string) string {
		upper := strings.ToUpper(m)
		// Don't replace if it's part of CURRENT_USER or SESSION_USER
		return strings.ReplaceAll(upper, "USER", "current_user()")
	})
}

// rewriteAlterTableMultiAction splits ALTER TABLE t a1, a2, a3 into
// separate ALTER TABLE statements that SQLite can execute one at a time.
func rewriteAlterTableMultiAction(sql string) string {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	if !strings.HasPrefix(upper, "ALTER TABLE") {
		return sql
	}
	// Find table name start
	rest := strings.TrimSpace(sql[len("ALTER TABLE"):])
	restUpper := strings.ToUpper(rest)
	// skip IF EXISTS
	if strings.HasPrefix(restUpper, "IF EXISTS ") {
		rest = strings.TrimSpace(rest[len("IF EXISTS "):])
	}
	// skip ONLY
	if strings.HasPrefix(strings.ToUpper(rest), "ONLY ") {
		rest = strings.TrimSpace(rest[len("ONLY "):])
	}
	// extract table name (possibly quoted)
	var tableName string
	if len(rest) > 0 && rest[0] == '"' {
		end := strings.Index(rest[1:], "\"")
		if end < 0 {
			return sql
		}
		tableName = rest[:end+2]
		rest = strings.TrimSpace(rest[end+2:])
	} else {
		i := 0
		for i < len(rest) && rest[i] != ' ' && rest[i] != '\t' && rest[i] != '\n' && rest[i] != ',' {
			i++
		}
		if i == 0 {
			return sql
		}
		tableName = rest[:i]
		rest = strings.TrimSpace(rest[i:])
	}
	// If no comma at the top level, nothing to split
	if !strings.Contains(rest, ",") {
		return sql
	}
	// Split actions by top-level comma
	actions := splitTopLevelCommas(rest)
	if len(actions) <= 1 {
		return sql
	}
	prefix := "ALTER TABLE " + tableName + " "
	var stmts []string
	for _, a := range actions {
		a = strings.TrimSpace(a)
		if a != "" {
			stmts = append(stmts, prefix+a)
		}
	}
	return strings.Join(stmts, "; ")
}

var (
	// Geometric operators not supported by SQLite — rewrite to FALSE (0)
	// ?# = path/lseg/box intersects
	geomIntersectsRE = regexp.MustCompile(`(?i)([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*)\s*\?\#\s*([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*)`)
)

// rewriteGeometricOperators rewrites PostgreSQL geometric operators to SQLite-compatible equivalents.
// These operators are not supported by SQLite and would cause syntax errors.
func rewriteGeometricOperators(sql string) string {
	if !strings.Contains(sql, "?#") {
		return sql
	}
	return geomIntersectsRE.ReplaceAllString(sql, "FALSE")
}

// updateTableAliasRE matches: UPDATE <table> [AS] <alias> SET (alias must not be a keyword)
var updateTableAliasRE = regexp.MustCompile(`(?i)^\s*UPDATE\s+([A-Za-z_][A-Za-z0-9_.]*)\s+(?:AS\s+)?([A-Za-z_][A-Za-z0-9_]*)\s+SET\s`)

var updateAliasKeywords = map[string]bool{
	"SET": true, "FROM": true, "WHERE": true, "RETURNING": true, "ONLY": true,
}

// rewriteUpdateAlias rewrites UPDATE table alias SET ... to UPDATE table SET ...
// and replaces alias.col references with tablename.col throughout the statement.
// SQLite does not support table aliases on the UPDATE target.
func rewriteUpdateAlias(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.HasPrefix(strings.TrimSpace(lower), "update ") {
		return sql
	}
	m := updateTableAliasRE.FindStringSubmatchIndex(sql)
	if m == nil {
		return sql
	}
	tableName := sql[m[2]:m[3]]
	aliasName := sql[m[4]:m[5]]
	if updateAliasKeywords[strings.ToUpper(aliasName)] {
		return sql
	}
	// Rebuild: UPDATE tableName SET <rest>
	setStart := m[1] - len("SET ") // position of SET keyword was consumed by the regex \s
	// m[1] points to just after "SET ", but we need to find where SET starts
	// The regex ends after "SET\s", so m[1] is the position after the trailing space
	// We need to replace from beginning up to "SET " with "UPDATE tableName SET "
	prefix := sql[:m[0]]
	rest := sql[m[1]:]
	// Replace alias.col with tableName.col in rest (case-insensitive alias match)
	_ = setStart
	aliasPrefix := aliasName + "."
	aliasPrefixLower := strings.ToLower(aliasPrefix)
	var result strings.Builder
	restLower := strings.ToLower(rest)
	i := 0
	for i < len(rest) {
		if strings.HasPrefix(restLower[i:], aliasPrefixLower) {
			// Check word boundary before alias
			if i == 0 || !isIdentByte(rest[i-1]) {
				result.WriteString(tableName)
				result.WriteByte('.')
				i += len(aliasPrefix)
				continue
			}
		}
		result.WriteByte(rest[i])
		i++
	}
	return prefix + "UPDATE " + tableName + " SET " + result.String()
}

// rewriteLimitParens removes outer parentheses from LIMIT/OFFSET expressions.
// SQLite does not support LIMIT (expr) — the parens must be removed.
func rewriteLimitParens(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "limit") && !strings.Contains(lower, "offset") {
		return sql
	}
	var result strings.Builder
	i := 0
	n := len(sql)
	for i < n {
		// Check for LIMIT( or OFFSET( pattern
		uLen := 0
		lowerRem := strings.ToLower(sql[i:])
		if strings.HasPrefix(lowerRem, "limit ") || strings.HasPrefix(lowerRem, "limit\t") || strings.HasPrefix(lowerRem, "limit\n") {
			uLen = 6
		} else if strings.HasPrefix(lowerRem, "offset ") || strings.HasPrefix(lowerRem, "offset\t") || strings.HasPrefix(lowerRem, "offset\n") {
			uLen = 7
		}
		if uLen > 0 {
			// Check if what follows (after optional whitespace) is '('
			j := i + uLen
			for j < n && (sql[j] == ' ' || sql[j] == '\t' || sql[j] == '\n') {
				j++
			}
			if j < n && sql[j] == '(' {
				// Find matching closing paren
				depth := 1
				k := j + 1
				inSingle := false
				for k < n && depth > 0 {
					if !inSingle && sql[k] == '\'' {
						inSingle = true
					} else if inSingle {
						if sql[k] == '\'' && k+1 < n && sql[k+1] == '\'' {
							k++
						} else if sql[k] == '\'' {
							inSingle = false
						}
					} else if sql[k] == '(' {
						depth++
					} else if sql[k] == ')' {
						depth--
					}
					k++
				}
				if depth == 0 {
					inner := strings.TrimSpace(sql[j+1 : k-1])
					innerLower := strings.ToLower(inner)
					// Don't strip if it's a subquery (SQLite needs the parens for SELECT)
					if strings.HasPrefix(innerLower, "select ") || strings.HasPrefix(innerLower, "with ") {
						result.WriteString(sql[i : k])
						i = k
						continue
					}
					// For CASE expressions: wrap with IFNULL(..., -1) so NULL LIMIT/OFFSET means no limit.
					// LIMIT CASE ... must have parens stripped (SQLite doesn't allow LIMIT (CASE ...))
					// OFFSET CASE ... must keep parens (SQLite doesn't allow OFFSET CASE ...)
					if strings.HasPrefix(innerLower, "case ") || innerLower == "case" {
						if uLen == 6 { // LIMIT
							result.WriteString(sql[i : i+uLen])
							result.WriteString("IFNULL(")
							result.WriteString(inner)
							result.WriteString(", -1)")
						} else { // OFFSET — keep parens, wrap inner with IFNULL
							result.WriteString(sql[i : i+uLen])
							result.WriteString("(IFNULL(")
							result.WriteString(inner)
							result.WriteString(", 0))")
						}
						i = k
						continue
					}
					// Write keyword + space
					result.WriteString(sql[i : i+uLen])
					// Write inner content (without outer parens)
					result.WriteString(inner)
					i = k
					continue
				}
			}
		}
		result.WriteByte(sql[i])
		i++
	}
	return result.String()
}

// rewriteUpdateDefault replaces DEFAULT in UPDATE SET clauses with NULL.
// SQLite does not support DEFAULT in UPDATE SET assignments.
func rewriteUpdateDefault(sql string) string {
	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "default") {
		return sql
	}
	trimmed := strings.TrimSpace(lower)
	if !strings.HasPrefix(trimmed, "update ") && !strings.HasPrefix(trimmed, "with ") {
		return sql
	}
	// Find SET keyword
	setIdx := indexKeywordOutsideQuotesAndParens(lower, "set")
	if setIdx == -1 {
		return sql
	}
	prefix := sql[:setIdx+3]
	suffix := sql[setIdx+3:]
	suffixLower := strings.ToLower(suffix)

	var result strings.Builder
	i := 0
	n := len(suffix)
	inSingle := false
	inDouble := false
	depth := 0
	for i < n {
		if inSingle {
			if suffix[i] == '\'' && i+1 < n && suffix[i+1] == '\'' {
				result.WriteString("''")
				i += 2
				continue
			}
			if suffix[i] == '\'' {
				inSingle = false
			}
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if inDouble {
			if suffix[i] == '"' && i+1 < n && suffix[i+1] == '"' {
				result.WriteString(`""`)
				i += 2
				continue
			}
			if suffix[i] == '"' {
				inDouble = false
			}
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if suffix[i] == '\'' {
			inSingle = true
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if suffix[i] == '"' {
			inDouble = true
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if suffix[i] == '(' {
			depth++
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if suffix[i] == ')' {
			depth--
			result.WriteByte(suffix[i])
			i++
			continue
		}
		if depth == 0 && i+7 <= n && strings.ToLower(suffixLower[i:i+7]) == "default" {
			leftBound := i == 0 || !isIdentByte(suffix[i-1])
			rightBound := i+7 == n || !isIdentByte(suffix[i+7])
			if leftBound && rightBound {
				result.WriteString("NULL")
				i += 7
				continue
			}
		}
		result.WriteByte(suffix[i])
		i++
	}
	return prefix + result.String()
}

// rewriteUpdateMultiColumn rewrites PostgreSQL multi-column assignment syntax:
//   UPDATE t SET (a, b, c) = (v1, v2, v3) ...
// to standard SQL:
//   UPDATE t SET a = v1, b = v2, c = v3 ...
// SQLite does not support the tuple assignment form.
func rewriteUpdateMultiColumn(sql string) string {
	lower := strings.ToLower(sql)
	trimmed := strings.TrimSpace(lower)
	if !strings.HasPrefix(trimmed, "update ") && !strings.HasPrefix(trimmed, "with ") {
		return sql
	}
	if !strings.Contains(lower, "set (") && !strings.Contains(lower, "set(") {
		return sql
	}
	setIdx := indexKeywordOutsideQuotesAndParens(lower, "set")
	if setIdx == -1 {
		return sql
	}

	prefix := sql[:setIdx+3]
	rest := sql[setIdx+3:]
	restLower := strings.ToLower(rest)
	_ = restLower

	var assignments []string
	i := 0
	n := len(rest)
	for i < n {
		for i < n && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == '\n' || rest[i] == ',') {
			i++
		}
		if i >= n {
			break
		}

		rem := strings.ToLower(rest[i:])
		if strings.HasPrefix(rem, "where ") || strings.HasPrefix(rem, "where\t") ||
			strings.HasPrefix(rem, "from ") || strings.HasPrefix(rem, "returning ") {
			break
		}

		if rest[i] == '(' {
			cols, colEnd := parseParenList(rest, i)
			if colEnd == -1 || len(cols) == 0 {
				return sql
			}
			j := colEnd
			for j < n && (rest[j] == ' ' || rest[j] == '\t' || rest[j] == '\n') {
				j++
			}
			if j >= n || rest[j] != '=' {
				return sql
			}
			j++
			for j < n && (rest[j] == ' ' || rest[j] == '\t' || rest[j] == '\n') {
				j++
			}
			if j >= n {
				return sql
			}
			if rest[j] == '(' {
				valLower := strings.ToLower(rest[j+1:])
				valLower = strings.TrimLeft(valLower, " \t\n")
				if strings.HasPrefix(valLower, "select ") {
					_, valEnd := readBalancedParen(rest, j)
					if valEnd == -1 {
						return sql
					}
					assignments = append(assignments, rest[i:valEnd])
					i = valEnd
					continue
				}
				vals, valEnd := parseParenList(rest, j)
				if valEnd == -1 || len(vals) != len(cols) {
					return sql
				}
				for idx, col := range cols {
					assignments = append(assignments, col+" = "+vals[idx])
				}
				i = valEnd
			} else {
				return sql
			}
		} else {
			end := findAssignmentEnd(rest, i)
			assignments = append(assignments, strings.TrimSpace(rest[i:end]))
			i = end
		}
	}

	if len(assignments) == 0 {
		return sql
	}

	suffix := findSuffixAfterAssignments(rest)
	return prefix + " " + strings.Join(assignments, ", ") + suffix
}

// parseParenList parses a parenthesized comma-separated list starting at pos.
// Returns (items, endPos) where endPos is the index after the closing ')'.
func parseParenList(s string, pos int) ([]string, int) {
	if pos >= len(s) || s[pos] != '(' {
		return nil, -1
	}
	var items []string
	var cur strings.Builder
	depth := 0
	inSingle := false
	inDouble := false
	i := pos
	n := len(s)
	for i < n {
		c := s[i]
		if inSingle {
			if c == '\'' && i+1 < n && s[i+1] == '\'' {
				cur.WriteByte(c)
				cur.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == '\'' {
				inSingle = false
			}
			cur.WriteByte(c)
			i++
			continue
		}
		if inDouble {
			if c == '"' && i+1 < n && s[i+1] == '"' {
				cur.WriteByte(c)
				cur.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == '"' {
				inDouble = false
			}
			cur.WriteByte(c)
			i++
			continue
		}
		switch c {
		case '\'':
			inSingle = true
			cur.WriteByte(c)
		case '"':
			inDouble = true
			cur.WriteByte(c)
		case '(':
			depth++
			if depth > 1 {
				cur.WriteByte(c)
			}
		case ')':
			depth--
			if depth == 0 {
				items = append(items, strings.TrimSpace(cur.String()))
				return items, i + 1
			}
			cur.WriteByte(c)
		case ',':
			if depth == 1 {
				items = append(items, strings.TrimSpace(cur.String()))
				cur.Reset()
			} else {
				cur.WriteByte(c)
			}
		default:
			cur.WriteByte(c)
		}
		i++
	}
	return nil, -1
}

// readBalancedParen returns the substring including balanced parens starting at pos.
func readBalancedParen(s string, pos int) (string, int) {
	if pos >= len(s) || s[pos] != '(' {
		return "", -1
	}
	depth := 0
	inSingle := false
	i := pos
	n := len(s)
	for i < n {
		c := s[i]
		if inSingle {
			if c == '\'' && i+1 < n && s[i+1] == '\'' {
				i += 2
				continue
			}
			if c == '\'' {
				inSingle = false
			}
			i++
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[pos : i+1], i + 1
			}
		}
		i++
	}
	return "", -1
}

// findAssignmentEnd finds the end of a single UPDATE assignment.
// Stops at a top-level comma or WHERE/FROM/RETURNING keyword.
func findAssignmentEnd(s string, start int) int {
	n := len(s)
	depth := 0
	inSingle := false
	inDouble := false
	i := start
	for i < n {
		c := s[i]
		if inSingle {
			if c == '\'' && i+1 < n && s[i+1] == '\'' {
				i += 2
				continue
			}
			if c == '\'' {
				inSingle = false
			}
			i++
			continue
		}
		if inDouble {
			if c == '"' && i+1 < n && s[i+1] == '"' {
				i += 2
				continue
			}
			if c == '"' {
				inDouble = false
			}
			i++
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return i
			}
		}
		if depth == 0 {
			rem := strings.ToLower(s[i:])
			if strings.HasPrefix(rem, "where ") || strings.HasPrefix(rem, "where\t") ||
				strings.HasPrefix(rem, "from ") || strings.HasPrefix(rem, "returning ") {
				return i
			}
		}
		i++
	}
	return n
}

// findSuffixAfterAssignments finds WHERE/FROM/RETURNING suffix after UPDATE SET assignments.
func findSuffixAfterAssignments(s string) string {
	n := len(s)
	depth := 0
	inSingle := false
	inDouble := false
	i := 0
	for i < n {
		c := s[i]
		if inSingle {
			if c == '\'' && i+1 < n && s[i+1] == '\'' {
				i += 2
				continue
			}
			if c == '\'' {
				inSingle = false
			}
			i++
			continue
		}
		if inDouble {
			if c == '"' && i+1 < n && s[i+1] == '"' {
				i += 2
				continue
			}
			if c == '"' {
				inDouble = false
			}
			i++
			continue
		}
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 {
			rem := strings.ToLower(s[i:])
			if strings.HasPrefix(rem, "where ") || strings.HasPrefix(rem, "where\t") ||
				strings.HasPrefix(rem, "from ") || strings.HasPrefix(rem, "returning ") {
				return " " + strings.TrimSpace(s[i:])
			}
		}
		i++
	}
	return ""
}
