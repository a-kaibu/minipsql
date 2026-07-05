# syntax=docker/dockerfile:1.7

FROM postgres:latest

WORKDIR /work/regress

RUN <<'EOF'
cat > /usr/local/bin/run-postgres-regress <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

pgroot="${POSTGRES_SRC:-/workspace/external/postgres}"
regress="$pgroot/src/test/regress"
sqldir="$regress/sql"
report_dir="${REPORT_DIR:-/workspace/reports/postgres-regress}"
host="${MINIPSQL_HOST:-host.docker.internal}"
port="${MINIPSQL_PORT:-15432}"
user="${MINIPSQL_USER:-mini}"
db="${MINIPSQL_DB:-default}"
parallelism="${REGRESS_PARALLELISM:-8}"

if [[ ! -d "$sqldir" ]]; then
  printf 'missing PostgreSQL regress SQL directory: %s\n' "$sqldir" >&2
  printf 'run: git submodule update --init --recursive\n' >&2
  exit 2
fi

mkdir -p "$report_dir"
rm -rf "$report_dir"/*
summary="$report_dir/summary.txt"
file_results="$report_dir/file-results.tsv"
per_file_dir="$report_dir/per-file"
mkdir -p "$per_file_dir"

mapfile -t sql_files < <(find "$sqldir" -maxdepth 1 -type f -name '*.sql' | sort)
if [[ "${#sql_files[@]}" -eq 0 ]]; then
  printf 'no SQL files found in %s\n' "$sqldir" >&2
  exit 2
fi

ready=0
for _ in $(seq 1 100); do
  if PGPASSWORD= psql -h "$host" -p "$port" -U "$user" "$db" -c 'select 1' >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.1
done
if [[ "$ready" -ne 1 ]]; then
  printf 'minipsql did not become ready at %s:%s\n' "$host" "$port" >&2
  exit 2
fi

run_one_file() {
  local file="$1"
  local rel="${file#"$regress"/}"
  local slug
  slug="$(printf '%s' "$rel" | tr '/' '_')"
  local out="$per_file_dir/${slug}.stdout"
  local err="$per_file_dir/${slug}.stderr"

  if (
    cd "$regress"
    PGPASSWORD= psql -X -h "$host" -p "$port" -U "$user" "$db" \
      -c '\set ON_ERROR_STOP off' \
      -c '\pset pager off' \
      -c '\timing off' \
      -f "$rel"
  ) >"$out" 2>"$err"; then
    : # success
  fi
}
export -f run_one_file
export regress host port user db per_file_dir

printf 'Running %d SQL files with parallelism=%s\n' "${#sql_files[@]}" "$parallelism"

printf '%s\n' "${sql_files[@]}" | \
  xargs -P "$parallelism" -I{} bash -c 'run_one_file "$@"' _ {}

success_files=0
failed_files=0
completed_files=0
total_errors=0
total_fatals=0

printf 'status\tfile\terror_count\tcompleted\n' >"$file_results"
for file in "${sql_files[@]}"; do
  rel="${file#"$regress"/}"
  slug="$(printf '%s' "$rel" | tr '/' '_')"
  err_file="$per_file_dir/${slug}.stderr"
  out_file="$per_file_dir/${slug}.stdout"

  file_error_count=0
  completed=0

  if [[ -f "$err_file" ]]; then
    file_error_count="$(grep -Ec '(^|: )(ERROR|FATAL|PANIC):' "$err_file" || true)"
    file_fatal_count="$(grep -Ec '(^|: )(FATAL|PANIC):' "$err_file" || true)"
    total_errors=$((total_errors + file_error_count))
    total_fatals=$((total_fatals + file_fatal_count))
  fi

  if [[ -f "$out_file" ]]; then
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

included_count="${#sql_files[@]}"
not_completed_files=$((included_count - completed_files))
overall_status=0
if [[ "$total_errors" -gt 0 || "$failed_files" -gt 0 ]]; then
  overall_status=1
fi

{
  printf 'postgres_regress_sql_dir=%s\n' "$sqldir"
  printf 'psql_runner=parallel-per-file\n'
  printf 'parallelism=%s\n' "$parallelism"
  printf 'total_files=%s\n' "$included_count"
  printf 'success_files=%s\n' "$success_files"
  printf 'failed_files=%s\n' "$failed_files"
  printf 'completed_files=%s\n' "$completed_files"
  printf 'not_completed_files=%s\n' "$not_completed_files"
  printf 'psql_exit_status=%s\n' "$overall_status"
  printf 'error_count=%s\n' "$total_errors"
  printf 'fatal_count=%s\n' "$total_fatals"
  printf 'file_results=%s\n' "$file_results"
  printf '\nFirst 200 errors (across all files):\n'
  find "$per_file_dir" -name '*.stderr' -exec grep -h -E '(^|: )(ERROR|FATAL|PANIC):' {} + 2>/dev/null | head -n 200 || true
} >"$summary"

printf '%s\n' "PostgreSQL regress run complete."
cat "$summary"

exit "$overall_status"
SCRIPT
chmod +x /usr/local/bin/run-postgres-regress
EOF

ENTRYPOINT ["run-postgres-regress"]
