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
	onlyRE             = regexp.MustCompile(`(?i)\bFROM\s+ONLY\s+`)
	eStringRE          = regexp.MustCompile(`(?i)\bE'`)
	trueRE             = regexp.MustCompile(`(?i)\bTRUE\b`)
	falseRE            = regexp.MustCompile(`(?i)\bFALSE\b`)
	ifNotExistsRE      = regexp.MustCompile(`(?i)\bIF\s+NOT\s+EXISTS\b`)
	ifExistsRE         = regexp.MustCompile(`(?i)\bIF\s+EXISTS\b`)
	deleteFromRE       = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+([A-Za-z0-9_\".]+)\s+([A-Za-z0-9_\"]+)\s+`)
	pgCatalogPrefixRE  = regexp.MustCompile(`(?i)\bpg_catalog\.([a-zA-Z0-9_]+)\b`)
	anonymousIndexRE   = regexp.MustCompile(`(?i)\bCREATE\s+(UNIQUE\s+)?INDEX\s+ON\s+([A-Za-z0-9_\".]+)\b`)
	circleColRE        = regexp.MustCompile(`(?i)\b\w+\s+circle\b`)
	asNullRE           = regexp.MustCompile(`(?i)\bAS\s+null\b`)
	tableColumnAliasRE = regexp.MustCompile(`(?i)\b(int2_tbl|int4_tbl|int8_tbl)\b\s*\)?\s*(?:AS\s+)?([A-Za-z0-9_]+)\s*\(\s*([A-Za-z0-9_, ]+)\s*\)`)
	tableKeywordRE     = regexp.MustCompile(`(?i)(^|[^a-zA-Z0-9_.])(?:([a-zA-Z0-9_]+)\s+)?\bTABLE\s+([A-Za-z0-9_]+)\b`)
	arrayConstructorRE = regexp.MustCompile(`(?i)\bARRAY\s*\[(.*?)\]`)
	usingLessRE         = regexp.MustCompile(`(?i)\busing\s+<`)
	usingGreaterRE      = regexp.MustCompile(`(?i)\busing\s+>`)
	asteriskTableRE     = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+)\*`)
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
	partitionByClauseRE = regexp.MustCompile(`(?i)\s+PARTITION\s+BY\s+(?:RANGE|LIST|HASH)\s*\(`)
	partitionOfRE       = regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+\S+\s+PARTITION\s+OF\s+`)
	attachPartitionRE   = regexp.MustCompile(`(?i)\bATTACH\s+PARTITION\b`)
	detachPartitionRE   = regexp.MustCompile(`(?i)\bDETACH\s+PARTITION\b`)
	generatedIdentityRE = regexp.MustCompile(`(?i)\s+GENERATED\s+(?:ALWAYS|BY\s+DEFAULT)\s+AS\s+IDENTITY(?:\s*\([^)]*\))?`)
	enforcedRE          = regexp.MustCompile(`(?i)\s+(?:NOT\s+)?ENFORCED\b`)
	tableoidRE          = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+\.)?tableoid\b`)
	ctidRE              = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_]+\.)?ctid\b`)
	onCommitRE          = regexp.MustCompile(`(?i)\s+ON\s+COMMIT\s+(?:DELETE\s+ROWS|DROP|PRESERVE\s+ROWS)`)
	inheritsRE          = regexp.MustCompile(`(?i)\s+INHERITS\s*\([^)]*\)`)
	generatedStoredRE   = regexp.MustCompile(`(?i)\s+GENERATED\s+ALWAYS\s+AS\s+\(([^)]+)\)\s+(?:STORED|VIRTUAL)`)
	generatedByDefaultStoredRE = regexp.MustCompile(`(?i)\s+GENERATED\s+BY\s+DEFAULT\s+AS\s+\(([^)]+)\)\s+STORED`)
	dropMultiTableRE   = regexp.MustCompile(`(?i)^\s*(DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?)([A-Za-z0-9_".,\s]+?)(\s+(?:CASCADE|RESTRICT))?\s*;?\s*$`)
	indexOpClassRE     = regexp.MustCompile(`(?i)\b([A-Za-z0-9_]+)\s+([A-Za-z0-9_]+_ops\b)(?:\s+ASC|\s+DESC)?`)
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
	indexWithOptsRE    = regexp.MustCompile(`(?i)(CREATE\s+(?:UNIQUE\s+)?INDEX\b.+?)\s+WITH\s*\([^)]*\)`)
	createIndexRE      = regexp.MustCompile(`(?i)(CREATE\s+(?:UNIQUE\s+)?INDEX\s+)(IF\s+NOT\s+EXISTS\s+|)([A-Za-z0-9_"]+)`)
	createAsExecuteRE  = regexp.MustCompile(`(?i)^\s*CREATE\s+(?:TEMP(?:ORARY)?\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z0-9_".]+)\s+AS\s+EXECUTE\b`)
	anyArrayRE         = regexp.MustCompile(`(?i)=\s*ANY\s*\(\s*ARRAY\s*\[([^\]]*)\](?:::[A-Za-z0-9_\[\]]+)?\s*\)`)
	namedArgRE         = regexp.MustCompile(`(?i)\b[A-Za-z_][A-Za-z0-9_]*\s*=>\s*`)
	forPortionOfRE     = regexp.MustCompile(`(?i)\s+FOR\s+PORTION\s+OF\s+\S+\s+FROM\s+\S+\s+TO\s+\S+`)
)

var idxCounter atomic.Int64


func Rewrite(sql string) string {
	out := rewriteTruncate(sql)
	out = rewriteCreateOrReplaceView(out)
	out = rewriteCreateView(out)
	out = rewriteCreateAsExecute(out)
	out = rewriteUnloggedTable(out)
	out = rewriteSelectInto(out)
	out = rewriteIndexConcurrently(out)
	out = rewriteFetchFirst(out)
	out = rewriteLateral(out)
	out = rewriteTablesample(out)
	out = rewriteExtract(out)
	out = pgCatalogCollateRE.ReplaceAllString(out, "")
	out = rewriteSchemaPrefix(out)
	out = rewriteViewOptions(out)
	out = rewriteTableWithOptions(out)
	out = rewriteCreateTableWithNoData(out)
	out = rewriteCreateTableColList(out)
	out = rewriteWithOrdinality(out)
	out = rewriteDropCascade(out)
	out = rewritePGCatalogOperator(out)
	out = rewriteGeneratedColumns(out)
	out = enforcedRE.ReplaceAllString(out, "")
	out = tableoidRE.ReplaceAllString(out, "0")
	out = ctidRE.ReplaceAllString(out, "'(0,1)'")
	out = onCommitRE.ReplaceAllString(out, "")
	out = inheritsRE.ReplaceAllString(out, "")
	out = rewritePartitionBy(out)
	out = rewriteOffsetBeforeLimit(out)
	out = rewriteBareOffset(out)
	out = rewriteDropMultiTable(out)
	out = rewriteIndexOpClasses(out)
	out = rewriteNullsFirstLast(out)
	out = rewriteForUpdate(out)
	out = rewriteIndexInclude(out)
	out = indexWithOptsRE.ReplaceAllString(out, "$1")
	out = rewriteCreateIndex(out)
	out = anyArrayRE.ReplaceAllString(out, "IN ($1)")
	out = namedArgRE.ReplaceAllString(out, "")
	out = forPortionOfRE.ReplaceAllString(out, "")
	out = rewriteCollateClause(out)
	out = rewriteColumnCompression(out)
	out = rewriteWithOids(out)
	out = rewriteEmptyTable(out)
	out = rewriteInsertDefault(out)
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
	out = arrayConstructorRE.ReplaceAllString(out, "minipsql_array($1)")
	out = usingLessRE.ReplaceAllString(out, "")
	out = usingGreaterRE.ReplaceAllString(out, "DESC")
	out = asteriskTableRE.ReplaceAllString(out, "$1")
	out = parenthesizedStartSelectRE.ReplaceAllString(out, "SELECT * FROM ($1)")
	out = parenthesizedSetSelectRE.ReplaceAllString(out, "$1 SELECT * FROM ($2)")
	out = rewriteStringContinuation(out)
	out = rewriteDollarQuotes(out)
	out = rewriteRegexFunctions(out)
	out = rewriteToBinOctHex(out)
	out = rewriteParametersAndCasts(out)
	out = serialRE.ReplaceAllString(out, "integer")
	out = boolRE.ReplaceAllString(out, "integer")
	out = nowRE.ReplaceAllString(out, "datetime('now')")
	out = quotedCharRE.ReplaceAllString(out, "::text")
	out = rewritePGCast(out)
	out = charLiteralRE.ReplaceAllString(out, "$1")
	out = pgLsnLiteralRE.ReplaceAllString(out, "$1")
	out = pointLiteralRE.ReplaceAllString(out, "$1")
	out = typedLiteralRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := typedLiteralRE.FindStringSubmatch(m)
		tName := strings.ToLower(sub[1])
		if tName == "char" || tName == "pg_lsn" || tName == "point" {
			return sub[2]
		}
		mapped := mapPGType(tName)
		return "CAST(" + sub[2] + " AS " + mapped + ")"
	})
	out = rewriteAnonymousIndex(out)
	out = createIndexUsingRE.ReplaceAllString(out, "$1 $2")
	out = ilikeRE.ReplaceAllString(out, "LIKE")
	out = rewriteRegexOps(out)
	out = rewriteSimilarTo(out)
	out = rewriteNullsOrder(out)
	out = rewriteGenerateSeries(out)
	out = rewritePosition(out)
	out = rewriteOverlay(out)
	out = rewriteSubstring(out)
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
	out = onlyRE.ReplaceAllString(out, "FROM ")
	out = eStringRE.ReplaceAllString(out, "'")
	out = asNullRE.ReplaceAllString(out, "AS \"null\"")
	out = rewriteStringAgg(out)
	out = rewriteArrayAgg(out)
	out = rewriteKeywords(out)
	return out
}

var (
	// FETCH FIRST n ROWS ONLY / FETCH NEXT n ROWS ONLY
	fetchFirstRE = regexp.MustCompile(`(?i)\bFETCH\s+(?:FIRST|NEXT)\s+(\d+)\s+ROWS?\s+(?:ONLY|WITH\s+TIES)\b`)
	// FETCH FIRST ROWS ONLY (no count = 1)
	fetchFirstNoCountRE = regexp.MustCompile(`(?i)\bFETCH\s+(?:FIRST|NEXT)\s+ROWS?\s+ONLY\b`)
	// OFFSET n ROWS
	offsetRowsRE = regexp.MustCompile(`(?i)\bOFFSET\s+(\d+)\s+ROWS\b`)
	// LATERAL keyword before subqueries/functions
	lateralRE = regexp.MustCompile(`(?i)\bLATERAL\s+`)
	// TABLESAMPLE clause
	tablesampleRE = regexp.MustCompile(`(?i)\bTABLESAMPLE\s+\w+\s*\([^)]*\)`)

	createOrReplaceViewRE = regexp.MustCompile(`(?i)^\s*CREATE\s+OR\s+REPLACE\s+(TEMP(?:ORARY)?\s+)?VIEW\s+`)
	createViewRE          = regexp.MustCompile(`(?i)(^\s*CREATE\s+(?:TEMP(?:ORARY)?\s+)?VIEW\s+)(IF\s+NOT\s+EXISTS\s+|)([A-Za-z0-9_".]+)`)
	createUnloggedTableRE = regexp.MustCompile(`(?i)\bCREATE\s+UNLOGGED\s+TABLE\b`)
	selectIntoRE          = regexp.MustCompile(`(?i)^\s*SELECT\s+(.+?)\s+INTO\s+(?:TEMP(?:ORARY)?\s+)?(?:TABLE\s+)?([A-Za-z0-9_".]+)\s+FROM\s+`)
	indexConcurrentlyRE   = regexp.MustCompile(`(?i)\b(CREATE\s+(?:UNIQUE\s+)?INDEX|DROP\s+INDEX)\s+CONCURRENTLY\s+`)
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

// rewriteFetchFirst rewrites SQL:2008 FETCH FIRST/NEXT syntax to LIMIT/OFFSET.
func rewriteFetchFirst(sql string) string {
	out := fetchFirstNoCountRE.ReplaceAllString(sql, "LIMIT 1")
	out = fetchFirstRE.ReplaceAllString(out, "LIMIT $1")
	out = offsetRowsRE.ReplaceAllString(out, "OFFSET $1")
	return out
}

// rewriteLateral strips the LATERAL keyword (SQLite evaluates correlated subqueries natively).
func rewriteLateral(sql string) string {
	return lateralRE.ReplaceAllString(sql, "")
}

// rewriteTablesample strips TABLESAMPLE clauses (not supported in SQLite).
func rewriteTablesample(sql string) string {
	return tablesampleRE.ReplaceAllString(sql, "")
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
		typeName = mapPGType(typeName)
		beforeCast := strings.TrimRightFunc(sql[:loc[0]], unicode.IsSpace)
		expr := extractCastExpr(beforeCast)
		prefix := beforeCast[:len(beforeCast)-len(expr)]
		suffix := trailing + sql[loc[1]:]
		if expr == "" {
			sql = prefix + suffix
		} else {
			sql = prefix + "CAST(" + expr + " AS " + typeName + ")" + suffix
		}
	}
	return sql
}

func splitCastTypeTrailingSQL(typeName string) (string, string) {
	lower := strings.ToLower(typeName)
	minIdx := -1
	for _, keyword := range []string{" placing ", " for ", " leading ", " trailing ", " both ", " from ", " where ", " order ", " group ", " limit ", " union ", " intersect ", " except ", " and ", " or ", " is ", " as "} {
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
		if i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') {
			i--
		}
		return sql[i:]
	}
	if sql[i] == ')' {
		depth := 1
		i--
		for i >= 0 && depth > 0 {
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

		if sql[i] == ';' {
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
		strings.HasPrefix(lower, "start transaction isolation level"):
		return "begin"
	case lower == "commit" || lower == "end":
		return "commit"
	case lower == "rollback" || lower == "abort" || strings.HasPrefix(lower, "rollback to "):
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
		if hasOuterParen {
			rewritten += ")"
		}
		
		sql = sql[:idx] + rewritten + sql[i:]
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
		
		hasCloseParen := strings.Contains(m[:strings.Index(strings.ToLower(m), aliasName)], ")")
		
		if numCols != 1 {
			msg := fmt.Sprintf("table %q has 1 columns available but %d columns specified", aliasName, numCols)
			ret := fmt.Sprintf("(SELECT minipsql_error('%s', '42601', '') FROM %s) %s", msg, tableName, aliasName)
			if hasCloseParen {
				ret += ")"
			}
			return ret
		}
		
		colName := strings.TrimSpace(colParts[0])
		ret := fmt.Sprintf("(SELECT f1 AS %s FROM %s) %s", colName, tableName, aliasName)
		if hasCloseParen {
			ret += ")"
		}
		return ret
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

// rewriteCreateTableWithNoData rewrites CREATE TABLE ... AS SELECT ... WITH NO DATA
// to CREATE TABLE ... AS SELECT ... LIMIT 0.
func rewriteCreateTableWithNoData(sql string) string {
	if !createTableWithNoDataRE.MatchString(sql) {
		return sql
	}
	return createTableWithNoDataRE.ReplaceAllString(sql, "LIMIT 0")
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
		m := regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+((?:IF\s+NOT\s+EXISTS\s+)?[A-Za-z0-9_"]+)\s+PARTITION\s+OF\s+.*$`).FindStringSubmatch(out)
		if m != nil {
			return "CREATE TABLE IF NOT EXISTS " + m[1] + " (_dummy integer)"
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
				for j < n && sql[j] >= '0' && sql[j] <= '9' {
					j++
				}
				if j > numStart {
					numStr := sql[numStart:j]
					// Skip optional ROWS/ROW
					k := j
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
					// Check what's after: if it's ) ; or end of clause, no LIMIT
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
	prefix := sub[1]   // "DROP TABLE " or "DROP TABLE IF EXISTS "
	tableList := sub[2]
	// sub[3] is cascade/restrict - we ignore it (strip it)

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







