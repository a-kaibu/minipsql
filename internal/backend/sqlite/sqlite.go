package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
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
	if err := initDatabase(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize postgres catalog tables: %w", err)
	}
	return &Backend{db: db}, nil
}

func initDatabase(ctx context.Context, db *sql.DB) error {
	// Create catalog tables directly in main schema
	ddls := []string{
		`CREATE TABLE IF NOT EXISTS main.pg_namespace (
			oid INTEGER PRIMARY KEY,
			nspname TEXT,
			nspowner INTEGER,
			nspacl TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_type (
			oid INTEGER PRIMARY KEY,
			typname TEXT,
			typnamespace INTEGER,
			typowner INTEGER,
			typlen INTEGER,
			typbyval INTEGER,
			typtype TEXT,
			typcategory TEXT,
			typispreferred INTEGER,
			typisdefined INTEGER,
			typdelim TEXT,
			typrelid INTEGER,
			typelem INTEGER,
			typarray INTEGER,
			typalign TEXT DEFAULT 'i',
			typstorage TEXT DEFAULT 'p',
			typnotnull INTEGER DEFAULT 0,
			typbasetype INTEGER DEFAULT 0,
			typtypmod INTEGER DEFAULT -1,
			typndims INTEGER DEFAULT 0,
			typcollation INTEGER DEFAULT 0,
			typdefaultbin TEXT,
			typacl TEXT,
			typdefault TEXT,
			typinput TEXT DEFAULT '-',
			typoutput TEXT DEFAULT '-',
			typreceive TEXT DEFAULT '-',
			typsend TEXT DEFAULT '-',
			typmodin TEXT DEFAULT '-',
			typmodout TEXT DEFAULT '-',
			typanalyze TEXT DEFAULT '-',
			typsubscript TEXT DEFAULT '-'
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_index (
			indexrelid INTEGER PRIMARY KEY,
			indrelid INTEGER,
			indnatts INTEGER,
			indnkeyatts INTEGER,
			indisunique INTEGER,
			indisprimary INTEGER,
			indisexclusion INTEGER,
			indimmediate INTEGER,
			indisclustered INTEGER,
			indisvalid INTEGER,
			indcheckxmin INTEGER,
			indisready INTEGER,
			indislive INTEGER,
			indisreplident INTEGER,
			indkey TEXT,
			indcollation TEXT,
			indclass TEXT,
			indoption TEXT,
			indexprs TEXT,
			indpred TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_constraint (
			oid INTEGER PRIMARY KEY,
			conname TEXT,
			connamespace INTEGER,
			contype TEXT,
			condeferrable INTEGER,
			condeferred INTEGER,
			convalidated INTEGER,
			conrelid INTEGER,
			contypid INTEGER,
			conindid INTEGER,
			conparentid INTEGER,
			confrelid INTEGER,
			confupdtype TEXT,
			confdeltype TEXT,
			confmatchtype TEXT,
			conislocal INTEGER,
			coninhcount INTEGER,
			connoinherit INTEGER,
			conkey TEXT,
			confkey TEXT,
			conpfeqop TEXT,
			conppeqop TEXT,
			conffeqop TEXT,
			conexclop TEXT,
			conbin TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_depend (
			classid INTEGER,
			objid INTEGER,
			objsubid INTEGER,
			refclassid INTEGER,
			refobjid INTEGER,
			refobjsubid INTEGER,
			deptype TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_shdepend (
			dbid INTEGER,
			classid INTEGER,
			objid INTEGER,
			objsubid INTEGER,
			refclassid INTEGER,
			refobjid INTEGER,
			deptype TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_init_privs (
			objoid INTEGER,
			classid INTEGER,
			objsubid INTEGER,
			privtype TEXT,
			initprivs TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_shmem_allocations_numa (
			name TEXT,
			size INTEGER,
			numa_node INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_proc (
			oid INTEGER PRIMARY KEY,
			proname TEXT,
			pronamespace INTEGER,
			proowner INTEGER,
			prolang INTEGER,
			procost REAL,
			prorows REAL,
			provariadic INTEGER,
			prosupport INTEGER,
			prokind TEXT,
			prosecdef INTEGER,
			proleakproof INTEGER,
			proisstrict INTEGER,
			proretset INTEGER,
			provolatile TEXT,
			proparallel TEXT,
			pronargs INTEGER,
			pronargdefaults INTEGER,
			prorettype INTEGER,
			proargtypes TEXT,
			proallargtypes TEXT,
			proargmodes TEXT,
			proargnames TEXT,
			proargdefaults TEXT,
			protrftypes TEXT,
			prosrc TEXT,
			probin TEXT,
			prosqlbody TEXT,
			proconfig TEXT,
			proacl TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_am (
			oid INTEGER PRIMARY KEY,
			amname TEXT,
			amhandler INTEGER,
			amtype TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_opclass (
			oid INTEGER PRIMARY KEY,
			opcmethod INTEGER,
			opcname TEXT,
			opcnamespace INTEGER,
			opcowner INTEGER,
			opcfamily INTEGER,
			opcintype INTEGER,
			opcdefault INTEGER,
			opckeytype INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_operator (
			oid INTEGER PRIMARY KEY,
			oprname TEXT,
			oprnamespace INTEGER,
			oprowner INTEGER,
			oprkind TEXT,
			oprcanmerge INTEGER,
			oprcanhash INTEGER,
			oprleft INTEGER,
			oprright INTEGER,
			oprresult INTEGER,
			oprcom INTEGER,
			oprnegate INTEGER,
			oprcode TEXT,
			oprrest TEXT,
			oprjoin TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_database (
			oid INTEGER PRIMARY KEY,
			datname TEXT,
			datdba INTEGER,
			encoding INTEGER,
			datcollate TEXT,
			datctype TEXT,
			datistemplate INTEGER,
			datallowconn INTEGER,
			datconnlimit INTEGER,
			datlastsysoid INTEGER,
			datfrozenxid INTEGER,
			datminmxid INTEGER,
			dattablespace INTEGER,
			datacl TEXT,
			dathasloginevt INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_collation (
			oid INTEGER PRIMARY KEY,
			collname TEXT,
			collnamespace INTEGER,
			collowner INTEGER,
			collprovider TEXT,
			collisascii INTEGER,
			collencoding INTEGER,
			collcollate TEXT,
			collctype TEXT,
			colliculocale TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_authid (
			oid INTEGER,
			rolname TEXT PRIMARY KEY,
			rolsuper INTEGER DEFAULT 0,
			rolinherit INTEGER DEFAULT 1,
			rolcreaterole INTEGER DEFAULT 0,
			rolcreatedb INTEGER DEFAULT 0,
			rolcanlogin INTEGER DEFAULT 1,
			rolreplication INTEGER DEFAULT 0,
			rolbypassrls INTEGER DEFAULT 0,
			rolconnlimit INTEGER DEFAULT -1,
			rolpassword TEXT,
			rolvaliduntil TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_language (
			oid INTEGER PRIMARY KEY,
			lanname TEXT,
			lanowner INTEGER,
			lanispl INTEGER DEFAULT 0,
			lanpltrusted INTEGER DEFAULT 0,
			lanplcallfoid INTEGER DEFAULT 0,
			laninline INTEGER DEFAULT 0,
			lanvalidator INTEGER DEFAULT 0,
			lanacl TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_prepared_statements (
			name TEXT,
			statement TEXT,
			prepare_time TEXT,
			parameter_types TEXT,
			result_types TEXT,
			from_sql INTEGER,
			generic_plans INTEGER,
			custom_plans INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_locks (
			locktype TEXT,
			database INTEGER,
			datoid INTEGER,
			relation INTEGER,
			page INTEGER,
			tuple INTEGER,
			virtualxid TEXT,
			transactionid INTEGER,
			classid INTEGER,
			objid INTEGER,
			objsubid INTEGER,
			virtualtransaction TEXT,
			pid INTEGER,
			mode TEXT,
			granted INTEGER,
			fastpath INTEGER,
			waitstart TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_activity (
			datid INTEGER,
			datname TEXT,
			pid INTEGER,
			leader_pid INTEGER,
			usesysid INTEGER,
			usename TEXT,
			application_name TEXT,
			client_addr TEXT,
			client_hostname TEXT,
			client_port INTEGER,
			backend_start TEXT,
			xact_start TEXT,
			query_start TEXT,
			state_change TEXT,
			wait_event_type TEXT,
			wait_event TEXT,
			state TEXT,
			backend_xid INTEGER,
			backend_xmin INTEGER,
			query_id INTEGER,
			query TEXT,
			backend_type TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_aggregate (
			aggfnoid INTEGER PRIMARY KEY,
			aggkind TEXT,
			aggnumdirectargs INTEGER,
			aggtransfn TEXT,
			aggfinalfn TEXT,
			aggcombinefn TEXT,
			aggserialfn TEXT,
			aggdeserialfn TEXT,
			aggmtransfn TEXT,
			aggminvtransfn TEXT,
			aggmfinalfn TEXT,
			aggfinalextra INTEGER,
			aggmfinalextra INTEGER,
			aggfinalmodify TEXT,
			aggmfinalmodify TEXT,
			aggsortop INTEGER,
			aggtranstype INTEGER,
			aggtransspace INTEGER,
			aggmtranstype INTEGER,
			aggmtransspace INTEGER,
			agginitval TEXT,
			aggminitval TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_cast (
			oid INTEGER PRIMARY KEY,
			castsource INTEGER,
			casttarget INTEGER,
			castfunc INTEGER,
			castcontext TEXT,
			castmethod TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_enum (
			oid INTEGER PRIMARY KEY,
			enumtypid INTEGER,
			enumsortorder REAL,
			enumlabel TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_trigger (
			oid INTEGER PRIMARY KEY,
			tgrelid INTEGER,
			tgparentid INTEGER,
			tgname TEXT,
			tgfoid INTEGER,
			tgtype INTEGER,
			tgenabled TEXT,
			tgisinternal INTEGER,
			tgownedbyconstraint INTEGER,
			tgconstrrelid INTEGER,
			tgconstrindid INTEGER,
			tgconstraint INTEGER,
			tgdeferrable INTEGER,
			tginitdeferred INTEGER,
			tgnargs INTEGER,
			tgattr TEXT,
			tgargs TEXT,
			tgqual TEXT,
			tgoldtable TEXT,
			tgnewtable TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_statistic_ext (
			oid INTEGER PRIMARY KEY,
			stxrelid INTEGER,
			stxname TEXT,
			stxnamespace INTEGER,
			stxowner INTEGER,
			stxstattarget INTEGER,
			stxkeys TEXT,
			stxkind TEXT,
			stxexprs TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_statistic_ext_data (
			stxoid INTEGER,
			stxdinherit INTEGER,
			stxdndistinct TEXT,
			stxddependencies TEXT,
			stxdmcv TEXT,
			stxdexpr TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_inherits (
			inhrelid INTEGER,
			inhparent INTEGER,
			inhseqno INTEGER,
			inhdetachpending INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_opfamily (
			oid INTEGER PRIMARY KEY,
			opfmethod INTEGER,
			opfname TEXT,
			opfnamespace INTEGER,
			opfowner INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_amop (
			oid INTEGER PRIMARY KEY,
			amopfamily INTEGER,
			amoplefttype INTEGER,
			amoprighttype INTEGER,
			amopstrategy INTEGER,
			amoppurpose TEXT,
			amopopr INTEGER,
			amopmethod INTEGER,
			amopsortfamily INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_amproc (
			oid INTEGER PRIMARY KEY,
			amprocfamily INTEGER,
			amproclefttype INTEGER,
			amprocrighttype INTEGER,
			amprocnum INTEGER,
			amproc INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_description (
			objoid INTEGER,
			classoid INTEGER,
			objsubid INTEGER,
			description TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_shdescription (
			objoid INTEGER,
			classoid INTEGER,
			description TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_ts_config (
			oid INTEGER PRIMARY KEY,
			cfgname TEXT,
			cfgnamespace INTEGER,
			cfgowner INTEGER,
			cfgparser INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_ts_dict (
			oid INTEGER PRIMARY KEY,
			dictname TEXT,
			dictnamespace INTEGER,
			dictowner INTEGER,
			dicttemplate INTEGER,
			dictinitoption TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_ts_parser (
			oid INTEGER PRIMARY KEY,
			prsname TEXT,
			prsnamespace INTEGER,
			prsstart INTEGER,
			prstoken INTEGER,
			prsend INTEGER,
			prsheadline INTEGER,
			prslextype INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_ts_template (
			oid INTEGER PRIMARY KEY,
			tmplname TEXT,
			tmplnamespace INTEGER,
			tmplinit INTEGER,
			tmpllexize INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_range (
			rngtypid INTEGER PRIMARY KEY,
			rngsubtype INTEGER,
			rngmultitypid INTEGER,
			rngcollation INTEGER,
			rngsubopc INTEGER,
			rngcanonical INTEGER,
			rngsubdiff INTEGER,
			rngconstruct2 INTEGER DEFAULT 0,
			rngconstruct3 INTEGER DEFAULT 0,
			rngmltconstruct0 INTEGER DEFAULT 0,
			rngmltconstruct1 INTEGER DEFAULT 0,
			rngmltconstruct2 INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_roles (
			oid INTEGER,
			rolname TEXT PRIMARY KEY,
			rolsuper INTEGER,
			rolinherit INTEGER,
			rolcreaterole INTEGER,
			rolcreatedb INTEGER,
			rolcanlogin INTEGER,
			rolreplication INTEGER,
			rolconnlimit INTEGER,
			rolpassword TEXT,
			rolvaliduntil TEXT,
			rolbypassrls INTEGER,
			rolconfig TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_tablespace (
			oid INTEGER PRIMARY KEY,
			spcname TEXT,
			spcowner INTEGER,
			spcacl TEXT,
			spcoptions TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_foreign_server (
			oid INTEGER PRIMARY KEY,
			srvname TEXT,
			srvowner INTEGER,
			srvfdw INTEGER,
			srvtype TEXT,
			srvversion TEXT,
			srvacl TEXT,
			srvoptions TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_foreign_data_wrapper (
			oid INTEGER PRIMARY KEY,
			fdwname TEXT,
			fdwowner INTEGER,
			fdwhandler INTEGER,
			fdwvalidator INTEGER,
			fdwacl TEXT,
			fdwoptions TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_conversion (
			oid INTEGER PRIMARY KEY,
			conname TEXT,
			connamespace INTEGER,
			conowner INTEGER,
			conforencoding INTEGER,
			contoencoding INTEGER,
			conproc INTEGER,
			condefault INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_publication (
			oid INTEGER PRIMARY KEY,
			pubname TEXT,
			pubowner INTEGER,
			puballtables INTEGER,
			pubinsert INTEGER,
			pubupdate INTEGER,
			pubdelete INTEGER,
			pubtruncate INTEGER,
			pubviaroot INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_subscription (
			oid INTEGER PRIMARY KEY,
			subdbid INTEGER,
			subskiplsn TEXT,
			subname TEXT,
			subowner INTEGER,
			subenabled INTEGER,
			subbinary INTEGER,
			substream INTEGER,
			subtwophasestate TEXT,
			subdisableonerr INTEGER,
			subpasswordrequired INTEGER,
			subrunasowner INTEGER,
			subfailover INTEGER,
			subconninfo TEXT,
			subslotname TEXT,
			subsynccommit TEXT,
			subpublications TEXT,
			suborigin TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_user_tables (
			relid INTEGER,
			schemaname TEXT,
			relname TEXT,
			seq_scan INTEGER,
			last_seq_scan TEXT,
			seq_tup_read INTEGER,
			idx_scan INTEGER,
			last_idx_scan TEXT,
			idx_tup_fetch INTEGER,
			n_tup_ins INTEGER,
			n_tup_upd INTEGER,
			n_tup_del INTEGER,
			n_tup_hot_upd INTEGER,
			n_tup_newpage_upd INTEGER,
			n_live_tup INTEGER,
			n_dead_tup INTEGER,
			n_mod_since_analyze INTEGER,
			n_ins_since_vacuum INTEGER,
			last_vacuum TEXT,
			last_autovacuum TEXT,
			last_analyze TEXT,
			last_autoanalyze TEXT,
			vacuum_count INTEGER,
			autovacuum_count INTEGER,
			analyze_count INTEGER,
			autoanalyze_count INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_all_tables (
			relid INTEGER,
			schemaname TEXT,
			relname TEXT,
			seq_scan INTEGER,
			last_seq_scan TEXT,
			seq_tup_read INTEGER,
			idx_scan INTEGER,
			last_idx_scan TEXT,
			idx_tup_fetch INTEGER,
			n_tup_ins INTEGER,
			n_tup_upd INTEGER,
			n_tup_del INTEGER,
			n_tup_hot_upd INTEGER,
			n_tup_newpage_upd INTEGER,
			n_live_tup INTEGER,
			n_dead_tup INTEGER,
			n_mod_since_analyze INTEGER,
			n_ins_since_vacuum INTEGER,
			last_vacuum TEXT,
			last_autovacuum TEXT,
			last_analyze TEXT,
			last_autoanalyze TEXT,
			vacuum_count INTEGER,
			autovacuum_count INTEGER,
			analyze_count INTEGER,
			autoanalyze_count INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stats (
			schemaname TEXT,
			tablename TEXT,
			attname TEXT,
			inherited INTEGER,
			null_frac REAL,
			avg_width INTEGER,
			n_distinct REAL,
			most_common_vals TEXT,
			most_common_freqs TEXT,
			histogram_bounds TEXT,
			correlation REAL,
			most_common_elems TEXT,
			most_common_elem_freqs TEXT,
			elem_count_histogram TEXT,
			range_length_histogram TEXT,
			range_empty_frac REAL,
			range_bounds_histogram TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_auth_members (
			oid INTEGER PRIMARY KEY,
			roleid INTEGER,
			member INTEGER,
			grantor INTEGER,
			admin_option INTEGER,
			inherit_option INTEGER,
			set_option INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_indexes (
			schemaname TEXT,
			tablename TEXT,
			indexname TEXT,
			tablespace TEXT,
			indexdef TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_views (
			schemaname TEXT,
			viewname TEXT,
			viewowner TEXT,
			definition TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_rules (
			schemaname TEXT,
			tablename TEXT,
			rulename TEXT,
			definition TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_user_functions (
			funcid INTEGER,
			schemaname TEXT,
			funcname TEXT,
			calls INTEGER,
			total_time REAL,
			self_time REAL
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_cursors (
			name TEXT,
			statement TEXT,
			is_holdable INTEGER,
			is_binary INTEGER,
			is_scrollable INTEGER,
			creation_time TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_prepared_xacts (
			"transaction" INTEGER,
			gid TEXT,
			prepared TEXT,
			owner TEXT,
			database TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_largeobject_metadata (
			oid INTEGER PRIMARY KEY,
			lomowner INTEGER,
			lomacl TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_statistic (
			starelid INTEGER,
			staattnum INTEGER,
			stainherit INTEGER,
			stanullfrac REAL,
			stawidth INTEGER,
			stadistinct REAL,
			stakind1 INTEGER,
			stakind2 INTEGER,
			stakind3 INTEGER,
			stakind4 INTEGER,
			stakind5 INTEGER,
			staop1 INTEGER,
			staop2 INTEGER,
			staop3 INTEGER,
			staop4 INTEGER,
			staop5 INTEGER,
			stacoll1 INTEGER,
			stacoll2 INTEGER,
			stacoll3 INTEGER,
			stacoll4 INTEGER,
			stacoll5 INTEGER,
			stanumbers1 TEXT,
			stanumbers2 TEXT,
			stanumbers3 TEXT,
			stanumbers4 TEXT,
			stanumbers5 TEXT,
			stavalues1 TEXT,
			stavalues2 TEXT,
			stavalues3 TEXT,
			stavalues4 TEXT,
			stavalues5 TEXT
		)`,
		// Partition catalog tables (always empty — minipsql doesn't support partitioning)
		`CREATE TABLE IF NOT EXISTS main.pg_partition_tree (
			relid INTEGER,
			parentrelid INTEGER,
			isleaf INTEGER,
			level INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_partition_ancestors (
			relid INTEGER
		)`,
		// System views (always empty stubs)
		`CREATE TABLE IF NOT EXISTS main.pg_available_extensions (
			name TEXT, default_version TEXT, installed_version TEXT, comment TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_available_extension_versions (
			name TEXT, version TEXT, installed INTEGER, superuser INTEGER,
			trusted INTEGER, relocatable INTEGER, schema TEXT, requires TEXT, comment TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_config (
			name TEXT, setting TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_file_settings (
			sourcefile TEXT, sourceline INTEGER, seqno INTEGER, name TEXT,
			setting TEXT, applied INTEGER, error TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_hba_file_rules (
			rule_number INTEGER, file_name TEXT, line_number INTEGER,
			type TEXT, database TEXT, user_name TEXT, address TEXT,
			netmask TEXT, auth_method TEXT, options TEXT, error TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_ident_file_mappings (
			map_number INTEGER, file_name TEXT, line_number INTEGER,
			map_name TEXT, sys_name TEXT, pg_username TEXT, error TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_backend_memory_contexts (
			type TEXT, name TEXT, ident TEXT, parent TEXT, level INTEGER,
			path TEXT, total_bytes INTEGER, total_nblocks INTEGER,
			free_bytes INTEGER, free_chunks INTEGER, used_bytes INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_slru (
			name TEXT, blks_zeroed INTEGER, blks_hit INTEGER, blks_read INTEGER,
			blks_written INTEGER, blks_exists INTEGER, flushes INTEGER,
			truncates INTEGER, stats_reset TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_wal (
			wal_records INTEGER, wal_fpi INTEGER, wal_bytes INTEGER,
			wal_buffers_full INTEGER, wal_write INTEGER, wal_sync INTEGER,
			wal_write_time INTEGER, wal_sync_time INTEGER, stats_reset TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_wal_receiver (
			pid INTEGER, status TEXT, receive_start_lsn TEXT, receive_start_tli INTEGER,
			written_lsn TEXT, flushed_lsn TEXT, received_tli INTEGER,
			last_msg_send_time TIMESTAMP, last_msg_receipt_time TIMESTAMP,
			latest_end_lsn TEXT, latest_end_time TIMESTAMP, slot_name TEXT,
			sender_host TEXT, sender_port INTEGER, conninfo TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_recovery (
			pid INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_timezone_abbrevs (
			abbrev TEXT, utc_offset TEXT, is_dst INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_timezone_names (
			name TEXT, abbrev TEXT, utc_offset TEXT, is_dst INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_wait_events (
			type TEXT, name TEXT, description TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_settings (
			name TEXT, setting TEXT, unit TEXT, category TEXT,
			short_desc TEXT, extra_desc TEXT, context TEXT, vartype TEXT,
			source TEXT, min_val TEXT, max_val TEXT, enumvals TEXT,
			boot_val TEXT, reset_val TEXT, sourcefile TEXT, sourceline INTEGER,
			pending_restart INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS main.pg_stat_io (
			backend_type TEXT, object TEXT, context TEXT, reads INTEGER,
			read_time INTEGER, writes INTEGER, write_time INTEGER,
			writebacks INTEGER, writeback_time INTEGER, extends INTEGER,
			extend_time INTEGER, op_bytes INTEGER, hits INTEGER, evictions INTEGER,
			reuses INTEGER, fsyncs INTEGER, fsync_time INTEGER, stats_reset TIMESTAMP
		)`,
	}

	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}

	// Create dynamic views in main schema
	views := []string{
		`CREATE VIEW IF NOT EXISTS main.pg_class AS
		SELECT
			10000 + rowid AS oid,
			name AS relname,
			2200 AS relnamespace,
			0 AS reltype,
			0 AS reloftype,
			10 AS relowner,
			0 AS relam,
			0 AS relfilenode,
			0 AS reltablespace,
			0 AS relpages,
			0.0 AS reltuples,
			0 AS relallvisible,
			0 AS reltoastrelid,
			0 AS relhasindex,
			0 AS relisshared,
			'p' AS relpersistence,
			CASE type WHEN 'view' THEN 'v' ELSE 'r' END AS relkind,
			0 AS relnatts,
			0 AS relchecks,
			0 AS relhasrules,
			0 AS relhastriggers,
			0 AS relhassubclass,
			0 AS relrowsecurity,
			0 AS relforcerowsecurity,
			0 AS relispopulated,
			'd' AS relreplident,
			0 AS relispartition,
			0 AS relrewrite,
			0 AS relfrozenxid,
			0 AS relminmxid,
			NULL AS reloptions,
			NULL AS relpartbound
		FROM sqlite_schema
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'pg_%'`,

		`CREATE VIEW IF NOT EXISTS main.pg_attribute AS
		SELECT 
			10000 + t.rowid AS attrelid,
			p.name AS attname,
			CASE 
				WHEN lower(p.type) = 'integer' THEN 23
				WHEN lower(p.type) = 'text' THEN 25
				ELSE 25
			END AS atttypid,
			-1 AS attlen,
			p.cid + 1 AS attnum,
			0 AS attcacheoff,
			-1 AS atttypmod,
			0 AS attbyval,
			'p' AS attstorage,
			'i' AS attalign,
			p."notnull" AS attnotnull,
			0 AS atthasdef,
			0 AS atthasmissing,
			'' AS attidentity,
			'' AS attgenerated,
			0 AS attisdropped,
			1 AS attislocal,
			0 AS attinhcount,
			0 AS attcollation
		FROM sqlite_schema t
		JOIN pragma_table_info(t.name) p
		WHERE t.type = 'table' AND t.name NOT LIKE 'sqlite_%'`,

		`CREATE VIEW IF NOT EXISTS main.pg_tables AS
		SELECT 
			'public' AS schemaname,
			name AS tablename,
			'mini' AS tableowner,
			NULL AS tablespace,
			0 AS hasindexes,
			0 AS hasrules,
			0 AS hastriggers,
			0 AS rowsecurity
		FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	}

	for _, view := range views {
		if _, err := db.ExecContext(ctx, view); err != nil {
			return err
		}
	}

	// Populate initial metadata
	populateSQL := `
		INSERT OR IGNORE INTO main.pg_namespace (oid, nspname) VALUES (2200, 'public');
		INSERT OR IGNORE INTO main.pg_namespace (oid, nspname) VALUES (11, 'pg_catalog');

		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (16, 'bool', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (17, 'bytea', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (20, 'int8', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (21, 'int2', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (23, 'int4', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (25, 'text', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (700, 'float4', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (701, 'float8', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1043, 'varchar', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1082, 'date', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1114, 'timestamp', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1184, 'timestamptz', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (1700, 'numeric', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (114, 'json', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (3802, 'jsonb', 11);
		INSERT OR IGNORE INTO main.pg_type (oid, typname, typnamespace) VALUES (2950, 'uuid', 11);

		INSERT OR IGNORE INTO main.pg_init_privs (objoid, classid, objsubid, privtype, initprivs) VALUES (0, 0, 0, 'i', '{}');
		INSERT OR IGNORE INTO main.pg_database (oid, datname, datdba, encoding, datcollate, datctype, datistemplate, datallowconn, datconnlimit) VALUES (1, 'default', 10, 6, 'C', 'C', 0, 1, -1);
		INSERT OR IGNORE INTO main.pg_database (oid, datname, datdba, encoding, datcollate, datctype, datistemplate, datallowconn, datconnlimit) VALUES (2, 'minipsql', 10, 6, 'C', 'C', 0, 1, -1);
		INSERT OR IGNORE INTO main.pg_database (oid, datname, datdba, encoding, datcollate, datctype, datistemplate, datallowconn, datconnlimit) VALUES (3, 'regression', 10, 6, 'C', 'C', 0, 1, -1);
	`
	if _, err := db.ExecContext(ctx, populateSQL); err != nil {
		return err
	}

	return nil
}

// NewSession returns a per-connection Backend that shares the underlying
// *sql.DB but has its own transaction state. Close is a no-op; the DB
// lifetime is managed by the owner Backend.
func (b *Backend) NewSession() backend.Backend {
	return &sessionBackend{db: b.db}
}

// sessionBackend wraps a shared *sql.DB with per-session transaction state.
type sessionBackend struct {
	db *sql.DB
	tx *sql.Tx
}

func (s *sessionBackend) queryer() interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *sessionBackend) execer() interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *sessionBackend) Query(ctx context.Context, query string, args []backend.Value) (*backend.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.queryer().QueryContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
		cols[i] = backend.Column{Name: normalizeSQLiteColName(name), TypeOID: compat.TypeOID(dbType)}
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

// normalizeSQLiteColName removes trailing () from function call column names.
// e.g. "txid_current()" → "txid_current", "current_user()" → "current_user"
func normalizeSQLiteColName(name string) string {
	if strings.HasSuffix(name, "()") {
		return name[:len(name)-2]
	}
	return name
}

func (s *sessionBackend) Exec(ctx context.Context, query string, args []backend.Value) (*backend.CommandTag, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := s.execer().ExecContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	return &backend.CommandTag{Tag: commandTag(query, n)}, nil
}

func (s *sessionBackend) Begin(ctx context.Context) error {
	if s.tx != nil {
		return errors.New("transaction already in progress")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	s.tx = tx
	return nil
}

func (s *sessionBackend) Commit(context.Context) error {
	if s.tx == nil {
		return errors.New("no transaction in progress")
	}
	err := s.tx.Commit()
	s.tx = nil
	return err
}

func (s *sessionBackend) Rollback(context.Context) error {
	if s.tx == nil {
		return errors.New("no transaction in progress")
	}
	err := s.tx.Rollback()
	s.tx = nil
	return err
}

func (s *sessionBackend) Close(context.Context) error { return nil }

func (s *sessionBackend) BulkInsert(ctx context.Context, table string, columns []string, rows [][]backend.Value) (int64, error) {
	b := &Backend{db: s.db, tx: s.tx}
	return b.BulkInsert(ctx, table, columns, rows)
}

func (s *sessionBackend) TableColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := s.queryer().QueryContext(ctx, "PRAGMA table_info(\""+table+"\")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("relation %q does not exist", table)
	}
	return cols, nil
}

func (b *Backend) Query(ctx context.Context, query string, args []backend.Value) (*backend.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
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
		cols[i] = backend.Column{Name: normalizeSQLiteColName(name), TypeOID: compat.TypeOID(dbType), Format: 0}
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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := b.execer().ExecContext(ctx, query, sqlArgs(args)...)
	if err != nil {
		return nil, err
	}
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
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

func (b *Backend) TableColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := b.queryer().QueryContext(ctx, "PRAGMA table_info(\""+table+"\")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("relation %q does not exist", table)
	}
	return cols, nil
}

func (b *Backend) BulkInsert(ctx context.Context, table string, columns []string, rows [][]backend.Value) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	if len(columns) == 0 {
		cols, err := b.TableColumns(ctx, table)
		if err != nil {
			return 0, err
		}
		columns = cols
	}

	colList := make([]string, len(columns))
	for i, c := range columns {
		colList[i] = "\"" + c + "\""
	}
	colSQL := strings.Join(colList, ", ")

	ncols := len(columns)
	placeholders := "(" + strings.Repeat("?, ", ncols-1) + "?)"

	const batchSize = 200
	var total int64
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO \"")
		sb.WriteString(table)
		sb.WriteString("\" (")
		sb.WriteString(colSQL)
		sb.WriteString(") VALUES ")
		for i := range batch {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(placeholders)
		}

		args := make([]any, 0, len(batch)*ncols)
		for _, row := range batch {
			for j := 0; j < ncols; j++ {
				if j < len(row) {
					switch row[j].Kind {
					case backend.ValueNull:
						args = append(args, nil)
					default:
						args = append(args, row[j].Value)
					}
				} else {
					args = append(args, nil)
				}
			}
		}

		res, err := b.execer().ExecContext(ctx, sb.String(), args...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}
