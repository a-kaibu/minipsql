# in-memory で本家 PostgreSQL を動かす方法の調査（2026-10-16 時点）

## 前提

- 「再実装やのうて本家の PostgreSQL をそのまま動かす」が条件。
  PostgreSQL 互換の別実装（pg-mem、doltgresql、CockroachDB など）は対象外。
- 言語は Go を主軸に読んだ（wasm2go への言及があるため）。
  Rust、Python、JS の場合も触れる。
- 「psql」はクライアントの名前やけど、ここではサーバー（postgres プロセス）の話として扱う。

## 結論（先出し）

- Go で in-memory の本家 PostgreSQL は、2026 年秋時点で「動く実装が複数ある」段階に来とる。
- テスト用途なら **pgmem**（shibukawa）がいちばん現実的。
  wasm2go で Go ソースに変換済み、CGo 不要、マルチ接続対応、起動 0.1 秒。
- アプリ組み込み用途なら **pgzero**（partite-ai）の設計が技術的にいちばん筋がええ。
  ただし生後 3 週間、ライセンス表記なし、スター 0。
- 「wasm ランタイム方式」と「wasm2go 方式」は、どっちも既に試された。
  差が出るんは実行速度と配布物の形で、互換性の差はほぼ無い。
  互換性を決めるんは、むしろ「fork をどう代替するか」のほう。

## 技術的な核心：fork をどう代替するか

PostgreSQL は接続ごとに postmaster が fork し、バックエンド同士は System V 共有メモリとシグナルでやりとりする。
WASM には fork も共有メモリもシグナルも無い。
せやから WASM 化は「fork の代替をどう作るか」で性格が決まる。

| 方式 | 採用例 | 同時接続 | 失うもの |
|---|---|---|---|
| single-user mode（`postgres --single`） | PGlite 本家、pglite-go、gopglite、hoop、pgwasm-postgres | 1 本 | 別接続からのロック待ち、デッドロック検出、LISTEN/NOTIFY、checkpointer、autovacuum、bgworker、parallel query |
| EXEC_BACKEND + 「モジュールインスタンス＝プロセス」 | pgmem、pgzero | 本物のマルチバックエンド | ホスト側に spawn、共有メモリ、セマフォ、シグナル、ソケットの実装が要る |
| ネイティブで子 postmaster を起動 | pglite-rs（Rust） | 本物 | WASM の利点（純 Go、クロスコンパイル）は無い |

EXEC_BACKEND は Windows 向けに元々ある仕組みで、fork の代わりに新しいプログラムイメージを起動して共有メモリに再接続する。
pgmem も pgzero もこれを流用して、各 PostgreSQL プロセスを WASM モジュールの別インスタンス（別 goroutine）として起動しとる。
共有メモリは、全インスタンスの線形メモリの同じアドレスに mmap（MAP_FIXED）で貼り付ける方式。

## 候補一覧

### A. ネイティブバイナリを子プロセスで起動する

- **fergusstrange/embedded-postgres**（Go、MIT、1.2k stars）
  - theseus-rs/postgresql-binaries からバイナリを落として起動。PostgreSQL 15〜18（18.6.0）。
  - v1.34.0 が安定版（2026-03）、v2 は alpha（2026-10-06）。Linux/macOS/Windows。
  - in-memory はドキュメントに無い。WorkDir や DataDir を tmpfs（/dev/shm）に置けば実質 in-memory になる（自分で設定する）。
  - 互換性は本物そのもの（ICU、OpenSSL、拡張、全部そのまま）。
  - 起動が遅い。pgmem 側の計測で first query まで約 5.1 秒。
- **pg_tmp（ephemeralpg）**：initdb 済みディレクトリを使い回すシェルツール。Go ライブラリやない。
- Rust なら theseus-rs/postgresql-embedded、Python なら tinypg や pgserver が同じ方式。

### B. PGlite（Emscripten ビルド）を WASM ランタイムで動かす

- **PGlite 本家**（electric-sql、Apache-2.0）
  - v0.5.x で PostgreSQL 18、最新 0.5.10（2026-10-14）。週 1000 万〜1300 万 DL。
  - single-user mode。接続は 1 本。pglite-socket で psql 等を多重化できるが「全ケース保証なし」と公式が書いとる。
  - 2026-03 の v0.4 で initdb を別 WASM プロセスに分離、PostGIS 対応、接続多重化（1 バックエンドへの多重化）。
  - ロードマップ：真のマルチ接続（バックエンドモデルを WASM でエミュレート）が「次の大物」、libpglite（ネイティブ版）は「長期目標」。
  - Electric 社は 2026-08 に Databricks（Neon チーム）へ合流。「PGlite はオープンソースのまま」とだけ明言。メンテ体制の明言は無し。Databricks 側は AI エージェントのサンドボックス用途を掲げとる。
  - pglite-build（WASI ビルド）と pglite-bindings（libpglite）は 2025 年夏から止まっとって、リポジトリに do-not-use タグ付き。
- **moriyoshi/pglite-go**（Go、Apache-2.0、9 stars、最終コミット 2026-10-13）
  - PGlite 0.5.5（PG 18.3）の Emscripten MAIN_MODULE（動的リンク）をそのまま使う。
  - バックエンド 3 種：wasmtime（CGo 必須、既定）、wazero（純 Go、`-tags wazero`）、wasm2go AOT（純 Go、`-tags aot`、2026-09-25 マージ、「feasibility track」扱い）。
  - 1 接続のみ。セッション状態（トランザクション、temp table、SET）が全呼び出しで共有される。COPY、LISTEN/NOTIFY なし。拡張なし。
  - in-memory VFS で動き、Close/Sync 時にホストへ保存。
  - wasmtime：warm start 0.2 秒、SELECT 1 本 150µs。
  - wazero：cold compile が 28 秒〜5 分（MAIN_MODULE の imported globals が wazero のコンパイラと相性悪い）、warm 4 秒。
  - AOT：Emscripten の longjmp がホスト側 throw なので、wasm2go に通すために独自の wasm→wasm 書き換えパス（longjmp 呼び出し後に `__THREW__` を見て早期 return するガード挿入）が必要やった。生成 Go 14.1M 行 / 221MB、wasm2go 実行に 90 秒 / 7GB RAM、go build 174 秒 / 2.5GB。実行性能は未計測。
  - ServeConn で psql から接続できる（1 クライアントがバックエンドを占有）。
- **bradfitz/gopglite**：PGlite WASI ビルド + wazero、PG16、コミット 1 個（2026-01）、README に「DO NOT USE」。
- **hoophq/hoop の gateway/pglite**：PGlite WASI（PG 17.x）+ wazero を製品に組み込んどる実例。1 セッション、プール上限 1、オンディスク、ファイル経由の転送。
- **navidrome PR #6084**（spike、未マージ）：PGlite WASI（PG 17.5）+ wazero。97k トラックのフルスキャンが 33 分、メモリ 600〜950MB。PGlite fork 側のマクロ stub が原因のメモリリークを発見して修正。API がスキャナのトランザクション待ちになって 5 倍遅くなるのを「この方式の主なコスト」と総括。

### C. wasm2go で Go ソースに変換する（ランタイム不要）

- **goccy/wasm2go**（MIT、v0.5.15、2026-09-05、89 stars、pre-1.0）
  - WASM → Go ソース + amd64/arm64 の Plan9 asm（他アーキは純 Go フォールバック）。
  - WASI preview1 のホスト実装を同梱。Emscripten の import は自前で用意する必要あり。
  - 2026-08 に例外処理（C++ 型）、SIMD、memory64、threads/shared memory 対応が入っとる。
  - 作者計測（13MB の SQL パーサ wasm、wazero 比）：cold compile 8.4 倍速、スカラー処理 18〜81 倍速、TPC-H のスキャン系は ±10%。
  - goccy 本人が go-googlesql v0.3.0 で実戦投入済み。
- **shibukawa/pgmem**（Go、MIT、117 commits、最終 2026-10-05、9 stars）
  - PostgreSQL 18.3（postgres-pglite fork）を Emscripten で**静的リンク**（動的リンク無し、JS 経由の longjmp 無し）でビルドし、wasm2go（shibukawa の fork）で Go に変換。実行時に wasm ランタイム無し、CGo 無し。
  - 2026-09-19 の PR #2 で single-user mode を捨て、EXEC_BACKEND + `-matomics` で postmaster + 接続ごとのバックエンドを再現。デッドロック検出（40P01）、pg_stat_activity、pg_locks、parallel query が動く。
  - in-memory のみ（ホストのディスクに一切書かない）。Snapshot と Fork でテストごとに DB を複製。
  - 数値（Apple M3）：Start 約 0.1 秒、fork 3.8ms、indexed SELECT 27µs（TCP）/ 8.5µs（in-process Dial）、200k 行ソート 117ms、RSS 約 150MB、バイナリ 36.7MB、生成 Go 115MB、初回 go build 約 35 秒。
  - Testcontainers 比較（同一マシン）：start to first query 70ms vs 1.09 秒、indexed SELECT 29µs vs 102µs。
  - 拡張 33 個（pgvector 0.8.6、pg_trgm、hstore、pgcrypto、uuid-ossp など）。ICU、OpenSSL、zlib 無し。TLS 無し、パスワード認証無し。32bit WASM なので 4GiB 上限。
  - Linux/macOS/Windows（CI は linux/amd64、darwin/arm64、windows/amd64）。
  - Python（PyPI）、Java（Maven）、Node（npm）向けに、同じバイナリをサブプロセスで動かすパッケージあり。
  - 作者自身が「本番環境には適さないテスト用のフェイク」と位置づけ。チューニングは「ストレージを in-memory にする以外やっていない」。
  - wasm2go fork 依存（`-chunks`、`-group-files`、jump table の純 Go フォールバック）。
  - Future Architect の記事（2026-10-15）に経緯あり。「WASM ランタイムは Go でも使えるが、そのままやとネイティブ比で性能が出ないので wasm2go で Go に変換」。

### D. PGlite に依存せず upstream を WASI でビルドして wazero で動かす

- **partite-ai/pgzero**（Go、ライセンス表記なし、6 commits 全部 2026-09-25、0 stars、作者 mpoindexter）
  - upstream REL_18_6 + 774 行のパッチ（18 ファイル）。wasi-sdk 34、wasm32-wasip2、component model（WIT で host interface 定義）。
  - postmaster、バックエンド、bgworker、parallel worker を全部別 wasm インスタンスとして wazero で実行。Go 側の「kernel」（11 ファイル）が spawn、シグナル、パイプ、ソケット、共有メモリ（512MiB 固定アドレスに mmap）、セマフォを提供。
  - in-memory（既定）か DataDir（ホストのディレクトリ）。pgx 経由の in-memory pipe、オプションで TCP（psql 用）。
  - 拡張：外部ライブラリ不要の contrib 全部 + pgvector 0.8.6（HNSW/IVFFlat）+ 独自 pgcrypto_lite。OpenSSL/ICU 無しなので pgcrypto、uuid-ossp、PostGIS は無し。
  - 起動「1 秒未満、プロセス初回は数秒」。wazero の fork（mpoindexter/wazero）で 2 倍速、go.mod に replace が要る。pgbench の数値は未公開。`make regress` はあるが結果の記載無し。
  - macOS/Linux のみ。Go 1.26.5。
  - 「upstream + 小さいパッチ」という保守性と、本物のプロセスモデルを両立しとる唯一の Go 実装。ただし実績ゼロ。

### E. ネイティブ静的ライブラリ（libpglite）

- **electric-sql/pglite-bindings**：libpglite の公式 WIP。最終コミット 2025-08-05、「draft」「do-not-use」。Python のゲートウェイだけ。Go バインディングは無し。
- **pglite-rs**（Rust、MIT、サードパーティ、v0.2.7 2026-06）：postgres-pglite fork を native static lib にして Rust から呼ぶ。単一セッション or 子 postmaster の multi-process。RSS 23MB（単一）/ 84MB（4 接続）。Go からは使えへん。

### F. x86 エミュレータ（v86）で Linux ごと動かす

- **supabase-community/postgres-wasm**：2022 年、PG13、ブラウザでデモする用。超遅い、巨大、メンテ気配無し。選択肢に入れん。

### G. Docker / Testcontainers

- 互換性は本物。tmpfs マウントで in-memory 化できる。Docker デーモン必須。pgmem 比較で start to first query 1.09 秒。

### H. その他

- **pgxsinkit/pgwasm-postgres**：upstream REL_18_6 + 5 パッチの Emscripten ビルド。PostgreSQL License。2026-09-27/28 に 80 commits。single-user。ブラウザ/Bun 向け。pg_regress 231 本中 178 pass（失敗の大半が single-user 由来）。
- **PostgreSQL 本体のマルチスレッド化**：2023 年の Heikki の提案以降、2026 年の着地は今回の検索では確認できず。PGlite の Sam Willis が実験ブランチを持っとる。これが入れば fork 問題は根本から消えるが、当てにできる時期やない。

## 「wasm ランタイム」vs「wasm2go」の比較

| 観点 | wazero | wasmtime | wasm2go |
|---|---|---|---|
| CGo | 不要 | 必要 | 不要 |
| 起動時のコンパイル | 秒〜分（動的リンクの PGlite で最悪 5 分、静的リンクなら 2.5 秒） | 約 1.4 秒、キャッシュで 0.03 秒 | 無し（go build に織り込み済み） |
| 実行速度 | 遅い（汎用ベンチで native の約 4.7 倍遅、pgmem 計測で wasm2go の 1/3〜1/4） | 速い（native の約 2.4 倍遅） | wazero 比でスカラー 2.7〜4 倍速、スキャン系は同等 |
| 配布物 | .wasm を同梱 | .wasm + libwasmtime | 生成 Go ソース 100〜220MB（リポジトリに入れるか companion module にする） |
| ビルド負荷 | 軽い | 軽い | 重い（pgmem 35 秒、pglite-go 174 秒 / 2.5GB、変換時に最大 7GB RAM） |
| ハマりどころ | fd_renumber 等の細かい非互換、imported globals で遅くなる | CGo のクロスコンパイル | Emscripten の host longjmp 非対応、pre-1.0、pgmem は fork 依存、Go 1.27 の `-S` 形式変更で asm 経路が壊れた実績 |

まとめると、
- 互換性は方式で変わらん。同じ wasm バイナリを動かすだけやから。
- 実行速度と「純 Go で配れるか」で wasm2go が勝つ。
- ビルドの重さと成熟度（pre-1.0、fork 依存）で wasm2go が負ける。
- wasm2go に通すなら、Emscripten の動的リンクと host longjmp は避けて静的リンクでビルドするんが正解（pgmem 方式）。pglite-go の AOT は動的リンク版を無理やり通した例で、その代償が 7GB RAM と 14M 行。

## 用途別の推奨

1. **テスト用の in-memory DB が欲しい**：pgmem。Go 以外（Python、Java、Node）も同じバイナリで。真のマルチ接続。
2. **アプリに組み込む（永続化あり、マルチ接続）**：pgzero の設計がいちばん正しい。採用するならライセンスを issue で確認してから。代替は embedded-postgres + tmpfs。
3. **互換性が最優先（ICU、SSL、PostGIS、本番と同一バイナリ）**：embedded-postgres か Docker。WASM 系はどれも ICU と OpenSSL を落としとる。
4. **1 接続で足りて、PGlite の拡張エコシステム（PostGIS 等）が欲しい**：pglite-go（wasmtime）。
5. **ブラウザ / JS**：PGlite 本家。
6. **Rust**：pglite-rs。

## minipsql を自前で作る場合の道筋

- 最短：pgmem か pgzero の薄いラッパーにする。どっちも Go モジュールとして import できる。
- 自前で WASM 化するなら、土台は 2 択。
  - pgzero 方式：upstream + wasi-sdk + 774 行パッチ + Go kernel。upstream 追従が楽。component model と wazero fork への依存あり。
  - pgmem 方式：postgres-pglite fork + Emscripten 静的リンク + wasm2go。PGlite のパッチ資産（pglitec.c 等）を使える。wasm2go fork への依存あり。
- どっちにしても、fork 代替（EXEC_BACKEND）、共有メモリの固定アドレス、シグナルの安全点配送、セマフォ、ソケット、/dev/urandom、`locale -a`、ICU/OpenSSL の無効化、32bit の 4GiB 上限、statement_timeout がホスト呼び出し間でしか効かん問題、あたりは必ず踏む。
- psql クライアント自体も WASM で欲しいなら、PGlite 系には pg_dump.wasm はあるが psql.wasm は配布物に無かった。別途ビルドが要る。

## 情報源

- PGlite v0.4 発表（2026-03-25）: https://electric.ax/blog/2026/03/25/announcing-pglite-v04
- PGlite 1000 万 DL（2026-06-25、ロードマップ）: https://electric.ax/blog/2026/06/25/pglite-reaches-10-million-weekly-downloads
- Electric が Databricks へ（2026-08-11）: https://electric.ax/blog/2026/08/11/electric-joining-databricks
- Databricks 側の発表: https://www.databricks.com/blog/electric-joins-databricks-bring-wasm-postgres-ai-agent-sandboxes
- PGlite PG18 対応の議論: https://github.com/electric-sql/pglite/discussions/766
- PGlite docs: https://pglite.dev/docs/ / https://pglite.dev/docs/pglite-socket / https://pglite.dev/extensions/
- pglite-build（WASI ビルド、停止中）: https://github.com/electric-sql/pglite-build
- pglite-bindings（libpglite、停止中）: https://github.com/electric-sql/pglite-bindings
- pgmem: https://github.com/shibukawa/pgmem / https://shibukawa.github.io/pgmem/ / PR #2 https://github.com/shibukawa/pgmem/pull/2
- Future Architect の pgmem 記事（2026-10-15）: https://future-architect.github.io/articles/20261015a/
- pgzero: https://github.com/partite-ai/pgzero
- pglite-go: https://github.com/moriyoshi/pglite-go / AOT PR https://github.com/moriyoshi/pglite-go/pull/1
- gopglite: https://github.com/bradfitz/gopglite
- hoop gateway/pglite: https://pkg.go.dev/github.com/hoophq/hoop/gateway/pglite
- navidrome spike: https://github.com/navidrome/navidrome/pull/6084
- wasm2go: https://github.com/goccy/wasm2go / goccy の go-googlesql 投稿 https://x.com/goccy54/status/2064909264645493224
- embedded-postgres: https://github.com/fergusstrange/embedded-postgres
- pglite-rs: https://lib.rs/crates/pglite-rs
- pgwasm-postgres: https://github.com/pgxsinkit/pgwasm-postgres
- postgres-wasm（v86）: https://github.com/supabase-community/postgres-wasm
- WASM ランタイム性能比較 2026: https://00f.net/2026/06/23/webassembly-runtimes-2026/
- マルチスレッド PostgreSQL 提案（2023）: https://www.postgresql.org/message-id/31cc6df9-53fe-3cd9-af5b-ac0d801163f4@iki.fi
- pg_tmp: https://eradman.com/ephemeralpg/
