#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
sqlancer="${SQLANCER_SRC:-$root/external/sqlancer}"
report_dir="${REPORT_DIR:-$root/reports/sqlancer}"
port="${MINIPSQL_PORT:-15432}"
host="${MINIPSQL_HOST:-127.0.0.1}"
user="${MINIPSQL_USER:-mini}"
db="${MINIPSQL_DB:-default}"
tries="${SQLANCER_TRIES:-1000}"

if [[ ! -x "${JAVA_HOME:-}/bin/java" && -x /usr/bin/java ]]; then
  java_path="$(readlink -f /usr/bin/java)"
  export JAVA_HOME="${java_path%/bin/java}"
fi

if [[ ! -d "$sqlancer" ]]; then
  printf 'missing SQLancer checkout: %s\n' "$sqlancer" >&2
  printf 'run: scripts/fetch-test-sources.sh\n' >&2
  exit 2
fi

mkdir -p "$report_dir" "$root/bin"
rm -f "$report_dir"/*

go build -o "$root/bin/minipsql" "$root/cmd/minipsql"
"$root/bin/minipsql" serve --host "$host" --port "$port" >"$report_dir/server.log" 2>&1 &
server_pid="$!"
cleanup() {
  kill "$server_pid" >/dev/null 2>&1 || true
  wait "$server_pid" >/dev/null 2>&1 || true
}
trap cleanup EXIT

ready=0
for _ in $(seq 1 100); do
  if PGPASSWORD= psql -h "$host" -p "$port" -U "$user" "$db" -c 'select 1' >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.1
done
if [[ "$ready" -ne 1 ]]; then
  printf 'minipsql did not become ready; see %s\n' "$report_dir/server.log" >&2
  exit 2
fi

if command -v mvn >/dev/null 2>&1 && command -v java >/dev/null 2>&1; then
  (cd "$sqlancer" && mvn -q -DskipTests package)
  jar="$(find "$sqlancer/target" -maxdepth 1 -type f -name 'sqlancer-*.jar' | sort | tail -n 1)"
  if [[ -z "$jar" ]]; then
    printf 'SQLancer jar was not produced under %s/target\n' "$sqlancer" >&2
    exit 2
  fi
  java -jar "$jar" --num-tries "$tries" postgres --host "$host" --port "$port" --username "$user" --database-name "$db" \
    >"$report_dir/sqlancer.stdout" 2>"$report_dir/sqlancer.stderr"
  exit $?
fi

if command -v docker >/dev/null 2>&1; then
  docker build -t minipsql-sqlancer "$sqlancer" >"$report_dir/docker-build.log" 2>&1
  docker run --rm --network host minipsql-sqlancer --num-tries "$tries" postgres --host "$host" --port "$port" --username "$user" --database-name "$db" \
    >"$report_dir/sqlancer.stdout" 2>"$report_dir/sqlancer.stderr"
  exit $?
fi

printf 'SQLancer requires java+maven or docker; neither is available.\n' >&2
exit 2
