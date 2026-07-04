#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pgroot="${POSTGRES_SRC:-$root/external/postgres}"
regress="$pgroot/src/test/regress"
sqldir="$regress/sql"
report_dir="${REPORT_DIR:-$root/reports/postgres-regress}"
port="${MINIPSQL_PORT:-15432}"
host="${MINIPSQL_HOST:-127.0.0.1}"
user="${MINIPSQL_USER:-mini}"
db="${MINIPSQL_DB:-default}"
psql_image="${POSTGRES_REGRESS_IMAGE:-postgres:latest}"

if [[ ! -d "$sqldir" ]]; then
  printf 'missing PostgreSQL regress SQL directory: %s\n' "$sqldir" >&2
  printf 'run: git submodule update --init --recursive\n' >&2
  exit 2
fi

mkdir -p "$report_dir"
rm -f "$report_dir"/*
summary="$report_dir/summary.txt"
stdout="$report_dir/psql.stdout"
stderr="$report_dir/psql.stderr"
file_results="$report_dir/file-results.tsv"

mapfile -t sql_files < <(find "$sqldir" -maxdepth 1 -type f -name '*.sql' | sort)
if [[ "${#sql_files[@]}" -eq 0 ]]; then
  printf 'no SQL files found in %s\n' "$sqldir" >&2
  exit 2
fi

go build -o "$root/bin/minipsql" "$root/cmd/minipsql"

server_log="$report_dir/server.log"
"$root/bin/minipsql" serve --host "$host" --port "$port" >"$server_log" 2>&1 &
server_pid="$!"
cleanup() {
  kill "$server_pid" >/dev/null 2>&1 || true
  wait "$server_pid" >/dev/null 2>&1 || true
}
trap cleanup EXIT

ready=0
for _ in $(seq 1 100); do
  if docker run --rm --network host -e PGPASSWORD= "$psql_image" \
    psql -h "$host" -p "$port" -U "$user" "$db" -c 'select 1' >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.1
done
if [[ "$ready" -ne 1 ]]; then
  printf 'minipsql did not become ready; see %s\n' "$server_log" >&2
  exit 2
fi

input="$report_dir/all-regress.sql"
{
  printf '\\set ON_ERROR_STOP off\n'
  printf '\\pset pager off\n'
  printf '\\timing off\n'
  for file in "${sql_files[@]}"; do
    rel="${file#"$regress"/}"
    printf '\\echo MINIPSQL_REGRESS_BEGIN %s\n' "$rel"
    printf '\\i %s\n' "$rel"
    printf '\\echo MINIPSQL_REGRESS_END %s\n' "$rel"
  done
} >"$input"

if (
  docker run --rm --network host \
    -e PGPASSWORD= \
    -v "$regress:/work/regress:ro" \
    -v "$report_dir:/work/report" \
    -w /work/regress \
    "$psql_image" \
    psql -X -h "$host" -p "$port" -U "$user" "$db" -f /work/report/all-regress.sql
) >"$stdout" 2>"$stderr"; then
  psql_status=0
else
  psql_status="$?"
fi

error_count="$(grep -Ec '(^|: )(ERROR|FATAL|PANIC):' "$stderr" || true)"
fatal_count="$(grep -Ec '(^|: )(FATAL|PANIC):' "$stderr" || true)"
included_count="${#sql_files[@]}"
last_begin="$(grep 'MINIPSQL_REGRESS_BEGIN' "$stdout" | tail -n 1 || true)"
last_end="$(grep 'MINIPSQL_REGRESS_END' "$stdout" | tail -n 1 || true)"
success_files=0
failed_files=0
completed_files=0

printf 'status\tfile\terror_count\tcompleted\n' >"$file_results"
for file in "${sql_files[@]}"; do
  rel="${file#"$regress"/}"
  file_error_count="$(awk -v prefix="psql:$rel:" 'index($0, prefix) == 1 && /(ERROR|FATAL|PANIC):/ { count++ } END { print count + 0 }' "$stderr")"
  completed=0
  if grep -Fxq "MINIPSQL_REGRESS_END $rel" "$stdout"; then
    completed=1
    completed_files=$((completed_files + 1))
  fi
  if [[ "$completed" -eq 1 && "$file_error_count" -eq 0 ]]; then
    status=ok
    success_files=$((success_files + 1))
  else
    status=failed
    failed_files=$((failed_files + 1))
  fi
  printf '%s\t%s\t%s\t%s\n' "$status" "$rel" "$file_error_count" "$completed" >>"$file_results"
done
not_completed_files=$((included_count - completed_files))

{
  printf 'postgres_regress_sql_dir=%s\n' "$sqldir"
  printf 'psql_runner=docker\n'
  printf 'psql_image=%s\n' "$psql_image"
  printf 'total_files=%s\n' "$included_count"
  printf 'success_files=%s\n' "$success_files"
  printf 'failed_files=%s\n' "$failed_files"
  printf 'completed_files=%s\n' "$completed_files"
  printf 'not_completed_files=%s\n' "$not_completed_files"
  printf 'psql_exit_status=%s\n' "$psql_status"
  printf 'error_count=%s\n' "$error_count"
  printf 'fatal_count=%s\n' "$fatal_count"
  printf 'last_begin=%s\n' "$last_begin"
  printf 'last_end=%s\n' "$last_end"
  printf 'file_results=%s\n' "$file_results"
  printf '\nFirst 200 errors:\n'
  grep -E '(^|: )(ERROR|FATAL|PANIC):' "$stderr" | head -n 200 || true
} >"$summary"

printf '%s\n' "PostgreSQL regress run complete."
cat "$summary"

if [[ "$psql_status" -ne 0 || "$error_count" -ne 0 ]]; then
  exit 1
fi
