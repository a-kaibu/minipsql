# minipsql SQLite Backend 実装計画

## 1. 目的

minipsql は、PostgreSQL 本体を起動せずに、PostgreSQL クライアントから接続できる軽量な in-memory テストDBを提供する。

初期実装では PostgreSQL 完全互換は目指さない。
PostgreSQL wire protocol を受け、内部では SQLite in-memory database を実行エンジンとして使う。

目標は以下。

```text
psql / pgx / node-postgres
        |
        | PostgreSQL wire protocol
        v
minipsql
        |
        | SQL compatibility layer
        v
SQLite in-memory backend
```

主用途は以下。

```text
- Docker不要の軽量PostgreSQL風テストDB
- CLIやライブラリの統合テスト用DB
- 一時的なSQL playground
- 複数DBインスタンスの同時起動
- CI上での高速なDBテスト
```

## 2. 非目標

初期段階では以下を目指さない。

```text
- PostgreSQL本体との完全互換
- PostgreSQLのMVCC完全再現
- WAL / replication / background worker
- PL/pgSQL
- extension互換
- trigger完全互換
- function / procedure完全互換
- role / permission完全互換
- Serializable isolation完全互換
- pg_catalog完全互換
- Prisma / Rails / Django の完全対応
```

minipsql は PostgreSQL-compatible database ではなく、PostgreSQL-wire-compatible lightweight in-memory test database として定義する。

## 3. 全体構成

初期構成は以下。

```text
minipsql
  ├── cmd/minipsql
  │    └── serve / query / test
  │
  ├── internal/pgwire
  │    └── PostgreSQL wire protocol handling
  │
  ├── internal/session
  │    └── connection/session state
  │
  ├── internal/compat
  │    ├── SQL rewrite
  │    ├── SHOW/SET handling
  │    ├── pg_catalog fake responses
  │    ├── information_schema fake responses
  │    └── type OID mapping
  │
  ├── internal/backend
  │    └── SQLite backend
  │
  └── internal/testsuite
       ├── hand-written tests
       ├── postgres regress subset
       └── later: SQLancer
```

基本方針は以下。

```text
1 database = 1 SQLite in-memory connection
1 PostgreSQL session = 1 minipsql session
```

最初は複数セッションで同一DBを共有する設計を避ける。
複数同時起動は、DBごとに独立した SQLite in-memory backend を持つことで実現する。

## 4. 初期CLI

最初に提供するCLIは `serve` のみでよい。

```bash
minipsql serve
```

デフォルト設定。

```text
host: 127.0.0.1
port: 15432
engine: sqlite
mode: in-memory
auth: trust
database: default
```

利用例。

```bash
psql -h localhost -p 15432 -U mini default
```

将来的なオプション。

```bash
minipsql serve --host 127.0.0.1 --port 15432
minipsql serve --db test
minipsql serve --engine sqlite
minipsql serve --shared-cache
minipsql serve --snapshot ./snapshot.sql
minipsql serve --init ./schema.sql
```

## 5. Phase 1: psqlで `select 1` を通す

### 目的

PostgreSQLクライアントから接続し、最小クエリを実行できる状態にする。

### 対応範囲

```sql
select 1;
```

### 実装内容

pgwire層で以下を処理する。

```text
client -> server:
- StartupMessage
- SSLRequest
- Query
- Terminate

server -> client:
- AuthenticationOk
- ParameterStatus
- BackendKeyData
- ReadyForQuery
- RowDescription
- DataRow
- CommandComplete
- ErrorResponse
```

最初は Simple Query Protocol のみ対応する。

```text
Query message
  -> SQL文字列を受け取る
  -> SQLite backendに投げる
  -> 結果をPostgreSQL wire形式で返す
```

### 成功条件

以下が成功する。

```bash
psql -h localhost -p 15432 -U mini default
```

```sql
select 1;
```

期待結果。

```text
 ?column?
----------
        1
```

## 6. Phase 2: 基本DDL/DML

### 目的

テストDBとして最低限使える状態にする。

### 対応SQL

```sql
create table users (
  id integer primary key,
  name text
);

insert into users values (1, 'alice');

select * from users;

update users set name = 'bob' where id = 1;

delete from users where id = 1;
```

### 実装内容

SQLite backendに以下のインターフェースを用意する。

```go
type Backend interface {
    Query(ctx context.Context, sql string, args []Value) (*Result, error)
    Exec(ctx context.Context, sql string, args []Value) (*CommandTag, error)
    Begin(ctx context.Context) error
    Commit(ctx context.Context) error
    Rollback(ctx context.Context) error
    Close(ctx context.Context) error
}
```

結果型。

```go
type Result struct {
    Columns []Column
    Rows    [][]Value
}

type Column struct {
    Name     string
    TypeOID  uint32
    Format   int16
}

type Value struct {
    Kind  ValueKind
    Value any
}
```

### 成功条件

psql上で以下が通る。

```sql
create table users(id integer primary key, name text);
insert into users values (1, 'alice');
select * from users;
update users set name = 'bob' where id = 1;
select * from users;
delete from users where id = 1;
select * from users;
```

## 7. Phase 3: PostgreSQLらしい接続初期化

### 目的

psqlや基本的なPostgreSQLドライバが期待する接続パラメータを返す。

### 対応する `ParameterStatus`

```text
server_version
server_version_num
client_encoding
DateStyle
integer_datetimes
standard_conforming_strings
TimeZone
application_name
```

推奨値。

```text
server_version = 14.0
server_version_num = 140000
client_encoding = UTF8
DateStyle = ISO, MDY
integer_datetimes = on
standard_conforming_strings = on
TimeZone = UTC
```

PostgreSQL本体ではないことを明示したい場合、`version()` では独自名を返す。

```text
PostgreSQL 14.0 compatible minipsql
```

### 対応SQL

```sql
select version();
show server_version;
show server_version_num;
show client_encoding;
show standard_conforming_strings;
show search_path;
select current_database();
select current_schema();
select current_user;
```

### 実装方針

これらは SQLite に投げず、compat layer で直接返す。

```text
Query
  -> compat.TryHandleBuiltin(sql)
      -> handledならfake result
      -> not handledならSQLiteへ
```

## 8. Phase 4: 型OIDとRowDescription

### 目的

PostgreSQLクライアント側の型デコードを壊さない。

### 最低限対応する型OID

```text
bool        16
bytea       17
int8        20
int2        21
int4        23
text        25
float4      700
float8      701
varchar     1043
date        1082
timestamp   1114
timestamptz 1184
numeric     1700
json        114
jsonb       3802
uuid        2950
```

SQLiteから返る型をPostgreSQL OIDにマッピングする。

初期マッピング。

```text
SQLite INTEGER -> int8 or int4
SQLite REAL    -> float8
SQLite TEXT    -> text
SQLite BLOB    -> bytea
SQLite NULL    -> text fallback
```

ただし SQLite は動的型付けなので、列定義から型を推定する仕組みも必要。

```text
query result
  -> column name
  -> table metadata lookup
  -> declared type
  -> PostgreSQL TypeOID
```

最初は簡易にすべて `text` でも動くが、pgx/node-postgres対応を考えると早めに OID を整える。

## 9. Phase 5: SQL rewrite 最小対応

### 目的

PostgreSQL方言のうち、頻出するものをSQLite向けに変換する。

### 初期対応

#### `$1` パラメータ

PostgreSQL。

```sql
select * from users where id = $1;
```

SQLite。

```sql
select * from users where id = ?;
```

#### `::` cast

PostgreSQL。

```sql
select $1::text;
```

SQLite。

```sql
select cast(? as text);
```

#### `serial`

PostgreSQL。

```sql
create table users (
  id serial primary key,
  name text
);
```

SQLite。

```sql
create table users (
  id integer primary key autoincrement,
  name text
);
```

#### `bool`

PostgreSQL。

```sql
active boolean
```

SQLite。

```sql
active integer
```

#### `now()`

PostgreSQL。

```sql
select now();
```

SQLite。

```sql
select datetime('now');
```

### 実装方針

初期は文字列ベースでよい。

```text
simple rewrite:
- $N -> ?
- ::type -> cast(...)
- serial -> integer
- boolean -> integer
```

ただし複雑なSQLで壊れやすいため、中期的には `auxten/postgresql-parser` で AST を見る。

段階。

```text
Phase 5A: simple regex/string rewrite
Phase 5B: parserでstatement種別を判定
Phase 5C: AST-based rewrite
Phase 5D: SQLite SQL printer
```

## 10. Phase 6: Extended Query Protocol

### 目的

psqlだけでなく、pgx、node-postgres、database/sql系ドライバの基本利用に対応する。

### 対応メッセージ

```text
Parse
Bind
Describe
Execute
Sync
Close
```

### セッション内状態

```go
type Session struct {
    ID string

    Database string
    User string

    TxState TxState

    PreparedStatements map[string]*PreparedStatement
    Portals map[string]*Portal

    Params map[string]string

    Backend Backend
}
```

PreparedStatement。

```go
type PreparedStatement struct {
    Name string
    SQL string
    ParamTypeOIDs []uint32
    RewrittenSQL string
}
```

Portal。

```go
type Portal struct {
    Name string
    StatementName string
    Args []Value
}
```

### 実行フロー

```text
Parse
  -> SQLを保存
  -> 必要ならrewrite

Bind
  -> 引数を保存
  -> portal作成

Describe
  -> RowDescriptionを返す

Execute
  -> backend.Query/Exec

Sync
  -> ReadyForQuery
```

### 成功条件

Go pgx で以下が通る。

```go
rows, err := conn.Query(ctx, "select $1::text", "hello")
```

node-postgres で以下が通る。

```js
await client.query("select $1::text", ["hello"])
```

## 11. Phase 7: transaction対応

### 目的

基本的なテストコードで使える transaction を提供する。

### 対応SQL

```sql
begin;
commit;
rollback;
```

別名も対応。

```sql
start transaction;
end;
abort;
```

### 実装方針

SQLite transaction に対応させる。

```text
BEGIN      -> SQLite BEGIN
COMMIT     -> SQLite COMMIT
ROLLBACK   -> SQLite ROLLBACK
```

セッション状態。

```text
Idle
InTransaction
FailedTransaction
```

PostgreSQL wire protocol の ReadyForQuery でも状態を返す。

```text
I = idle
T = in transaction
E = failed transaction
```

### 非対応

初期では以下は非対応。

```sql
savepoint;
rollback to savepoint;
release savepoint;
set transaction isolation level serializable;
```

## 12. Phase 8: information_schema 最小対応

### 目的

ORMやGUIツールの初期 introspection を少し通す。

### 対応対象

```sql
select * from information_schema.tables;
select * from information_schema.columns;
```

### 実装方針

SQLite metadata から fake information_schema を生成する。

テーブル一覧。

```sql
select name from sqlite_master where type = 'table';
```

列情報。

```sql
pragma table_info('<table>');
```

これを以下のようなPostgreSQL風レスポンスに変換する。

```text
information_schema.tables:
- table_catalog
- table_schema
- table_name
- table_type

information_schema.columns:
- table_catalog
- table_schema
- table_name
- column_name
- ordinal_position
- column_default
- is_nullable
- data_type
```

最初は必要な列だけ埋め、不明な列は空文字またはNULLにする。

## 13. Phase 9: pg_catalog 最小対応

### 目的

PostgreSQLドライバや軽量ORMが期待する最小カタログを返す。

### 優先対応

```text
pg_catalog.pg_namespace
pg_catalog.pg_class
pg_catalog.pg_attribute
pg_catalog.pg_type
pg_catalog.pg_tables
```

### 方針

完全なPostgreSQL catalogは作らない。
SQLite schema から必要な範囲だけfake catalogを生成する。

型情報は固定テーブルとして持つ。

```go
var BuiltinTypes = []PgType{
    {OID: 16, Name: "bool"},
    {OID: 20, Name: "int8"},
    {OID: 21, Name: "int2"},
    {OID: 23, Name: "int4"},
    {OID: 25, Name: "text"},
    {OID: 700, Name: "float4"},
    {OID: 701, Name: "float8"},
    {OID: 1043, Name: "varchar"},
    {OID: 1700, Name: "numeric"},
}
```

OIDは実体の永続性を保証しない。
in-memory test DBなので、セッション内で安定していればよい。

## 14. Phase 10: 初期化SQLとスナップショット

### 目的

テスト用途で使いやすくする。

### 初期化SQL

```bash
minipsql serve --init schema.sql
```

起動時に実行。

```text
create backend
  -> execute init SQL
  -> start pgwire server
```

### スナップショット

将来的に以下を提供する。

```bash
minipsql dump > snapshot.sql
minipsql serve --init snapshot.sql
```

あるいはSQLiteのbackup APIを使う。

初期段階ではSQL dumpで十分。

## 15. テスト方針

### Level 1: hand-written tests

最初に書くテスト。

```text
- startup handshake
- select 1
- create table
- insert
- select
- update
- delete
- transaction begin/commit/rollback
- version()
- show server_version
```

### Level 2: psql integration tests

実際に `psql` を起動して確認する。

```bash
psql -h localhost -p 15432 -U mini default -c "select 1"
```

### Level 3: driver tests

対象。

```text
- pgx
- database/sql + pgx stdlib
- node-postgres
```

最初の目標は pgx。

### Level 4: PostgreSQL regression subset

PostgreSQL本体の `src/test/regress/sql` から小さいものを選ぶ。

候補。

```text
select.sql の一部
create_table.sql の一部
insert.sql の一部
update.sql の一部
delete.sql の一部
join.sql の一部
aggregates.sql の一部
transactions.sql の一部
```

期待結果はPostgreSQL本体の expected をそのまま使わない。
minipsql用に期待結果を作る。

理由。

```text
- エラー文が違う
- 型表示が違う
- timestampが違う
- NOTICEが違う
- OIDが違う
- SQLite由来の挙動差がある
```

### Level 5: SQLancer

後期に導入する。
初期から導入すると未対応SQLでほぼ落ちるため、対応SQL grammarを絞って使う。

目的は以下。

```text
- crashしないこと
- panicしないこと
- SQLiteとの差分を検知すること
- minipsqlのrewriteバグを検知すること
```

## 16. 互換性ポリシー

minipsqlは互換性を明示的にレベル分けする。

```text
Level 0:
  psqlでselect 1が動く

Level 1:
  基本DDL/DMLが動く

Level 2:
  pgx/node-postgresの基本queryが動く

Level 3:
  transactionとprepared statementが動く

Level 4:
  information_schemaの一部が動く

Level 5:
  軽量ORMの簡単なschema introspectionが動く
```

READMEには必ず非対応リストを書く。

```text
Unsupported:
- PostgreSQL extension
- PL/pgSQL
- trigger
- advanced index
- advanced transaction isolation
- replication
- role/permission
- full pg_catalog compatibility
```

## 17. エラー処理

PostgreSQLクライアントは SQLSTATE を見ることがある。
そのため、SQLiteエラーをPostgreSQL風の ErrorResponse に変換する。

初期マッピング。

```text
SQLite syntax error
  -> 42601 syntax_error

table not found
  -> 42P01 undefined_table

column not found
  -> 42703 undefined_column

constraint failed
  -> 23505 unique_violation or 23514 check_violation

generic error
  -> XX000 internal_error
```

ErrorResponseには最低限以下を含める。

```text
Severity
SQLSTATE
Message
```

## 18. ディレクトリ構成案

```text
.
├── cmd
│   └── minipsql
│       └── main.go
│
├── internal
│   ├── server
│   │   └── server.go
│   │
│   ├── pgwire
│   │   ├── handler.go
│   │   ├── rowdesc.go
│   │   └── error.go
│   │
│   ├── session
│   │   ├── session.go
│   │   └── statement.go
│   │
│   ├── backend
│   │   ├── backend.go
│   │   └── sqlite
│   │       ├── sqlite.go
│   │       ├── convert.go
│   │       └── metadata.go
│   │
│   ├── compat
│   │   ├── builtin.go
│   │   ├── rewrite.go
│   │   ├── catalog.go
│   │   ├── information_schema.go
│   │   ├── oid.go
│   │   └── errors.go
│   │
│   └── testsuite
│       └── regress.go
│
├── testdata
│   ├── sql
│   └── expected
│
├── go.mod
└── README.md
```

## 19. マイルストーン

### M1: psql接続

```text
- pgwire server起動
- psql接続
- select 1
```

完了条件。

```bash
psql -h localhost -p 15432 -U mini default -c "select 1"
```

### M2: SQLite基本実行

```text
- create table
- insert
- select
- update
- delete
```

完了条件。

```sql
create table users(id integer primary key, name text);
insert into users values (1, 'alice');
select * from users;
```

### M3: PostgreSQL風builtin

```text
- select version()
- show server_version
- show search_path
- current_database()
- current_schema()
```

### M4: 型OID

```text
- int/text/bool/floatのOID
- RowDescription整備
- pgxでselectが通る
```

### M5: Extended Query Protocol

```text
- Parse
- Bind
- Describe
- Execute
- Sync
- $1 parameter
```

完了条件。

```go
conn.Query(ctx, "select $1::text", "hello")
```

### M6: transaction

```text
- begin
- commit
- rollback
- ReadyForQuery transaction status
```

### M7: information_schema

```text
- information_schema.tables
- information_schema.columns
```

### M8: pg_catalog最小対応

```text
- pg_type
- pg_class
- pg_attribute
- pg_namespace
```

### M9: テスト整備

```text
- hand-written tests
- psql integration tests
- pgx integration tests
- postgres regress subset
```

## 20. 最初の実装順

最短で価値を出すなら、この順番にする。

```text
1. psql-wireを使ってserverを立てる
2. SQLite in-memory backendをつなぐ
3. select 1を通す
4. create/insert/selectを通す
5. select version() / show server_version をcompatで返す
6. RowDescriptionの型OIDを整える
7. $1 -> ? のrewriteを入れる
8. Extended Query Protocolを入れる
9. pgxで接続テストする
10. information_schema.columnsを足す
```

## 21. 技術判断

### SQLite backendを選ぶ理由

```text
- in-memory databaseが自然
- 起動が速い
- 複数同時起動しやすい
- OLTP風のCREATE/INSERT/UPDATE/DELETEに向く
- Goから扱いやすい
- テストDB用途に合う
- DuckDBより小さな実装で始められる
```

### DuckDBを初期採用しない理由

```text
- 分析用途には強いが、軽量テストDB用途ではSQLiteの方が自然
- PostgreSQL互換で必要なcatalog/pgwire/type対応は結局自前
- INSERT/UPDATE/DELETE中心のユースケースではSQLiteの方が素直
```

DuckDB backendは将来的に追加する。

```bash
minipsql serve --engine duckdb
```

## 22. リスク

### PostgreSQL互換の範囲が膨らむ

対策。

```text
- READMEに非対応を明記する
- compatibility levelを定義する
- ORM完全対応を初期目標にしない
```

### SQL rewriteが壊れやすい

対策。

```text
- 最初は対応構文を限定する
- rewrite前後をテストする
- 中期的にPostgreSQL parserを導入する
```

### pg_catalog対応が重い

対策。

```text
- 最初はinformation_schemaを優先する
- pg_catalogはよく使われるクエリだけspecial-caseする
- DoltgreSQL/CockroachDBを参考にするが、完全互換は目指さない
```

### SQLiteとPostgreSQLの挙動差

例。

```text
- 型システム
- boolean
- timestamp
- numeric
- serial
- constraint error
- transaction isolation
- NULL sorting
```

対策。

```text
- 互換差分をドキュメント化する
- テストDB用途に絞る
- 必要な差分から順に吸収する
```

## 23. READMEでの説明案

minipsql は PostgreSQL 本体ではありません。

minipsql は、PostgreSQL wire protocol で接続できる軽量な in-memory test database です。
内部エンジンには SQLite を使用し、PostgreSQL クライアントから扱いやすいように SQL 方言、型OID、information_schema の一部を互換化します。

主な用途は、Docker を起動せずに PostgreSQL 風のテストDBを素早く用意することです。

```bash
minipsql serve
psql -h localhost -p 15432 -U mini default
```

対応範囲は段階的に拡張します。
PostgreSQL拡張、PL/pgSQL、replication、完全なpg_catalog互換は対象外です。

## 24. 最小PoCの完成定義

最初のPoCは以下を満たせば完成。

```bash
minipsql serve
```

別ターミナル。

```bash
psql -h localhost -p 15432 -U mini default
```

psql上。

```sql
select version();

create table users (
  id integer primary key,
  name text
);

insert into users values (1, 'alice');

select * from users;
```

期待される状態。

```text
- psqlから接続できる
- select結果が表示される
- create/insert/selectが動く
- DBはin-memoryでプロセス終了時に消える
- 複数DBを独立して立ち上げられる設計になっている
```

この状態まで行けば、minipsqlの方向性は十分に検証できる。

