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

mapfile -t sql_files < <(find "$sqldir" -maxdepth 1 -type f -name '*.sql' | sort)
if [[ "${#sql_files[@]}" -eq 0 ]]; then
  printf 'no SQL files found in %s\n' "$sqldir" >&2
  exit 2
fi

# Apply filter if set
filter_pattern="${REGRESS_FILTER:-${FILTER:-}}"
if [[ -n "$filter_pattern" ]]; then
  glob_pattern="${filter_pattern//%/*}"
  glob_pattern="${glob_pattern//_/?}"
  if [[ "$glob_pattern" != *'*'* && "$glob_pattern" != *'?'* ]]; then
    glob_pattern="*${glob_pattern}*"
  fi
  filtered=()
  for file in "${sql_files[@]}"; do
    name="$(basename "$file" .sql)"
    shopt -s nocasematch
    if [[ "$name" == $glob_pattern ]]; then
      filtered+=("$file")
    fi
    shopt -u nocasematch
  done
  sql_files=("${filtered[@]}")
  if [[ "${#sql_files[@]}" -eq 0 ]]; then
    printf 'no SQL files matched filter: %s\n' "$filter_pattern" >&2
    exit 0
  fi
fi

export PG_ABS_SRCDIR="$regress"
export PG_LIBDIR="$regress"
export PG_DLSUFFIX=".so"

if [[ -n "${REGRESS_BATCH:-}" && -n "${REGRESS_BATCH_COUNT:-}" ]]; then
  total="${#sql_files[@]}"
  batch_size=$(( (total + REGRESS_BATCH_COUNT - 1) / REGRESS_BATCH_COUNT ))
  start=$(( REGRESS_BATCH * batch_size ))
  sql_files=( "${sql_files[@]:$start:$batch_size}" )
  printf 'Batch %d/%d: files %d-%d of %d\n' \
    "$REGRESS_BATCH" "$REGRESS_BATCH_COUNT" \
    "$start" "$(( start + ${#sql_files[@]} - 1 ))" "$total"
  report_dir="${report_dir}/batch-${REGRESS_BATCH}"
fi

mkdir -p "$report_dir"
rm -rf "$report_dir"/* 2>/dev/null || true
summary="$report_dir/summary.txt"
file_results="$report_dir/file-results.tsv"
per_file_dir="$report_dir/per-file"
mkdir -p "$per_file_dir"

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
  slug="$(printf '%s' "$rel" | tr '/.' '__')"
  local dbname="regress_${slug}"
  local out="$per_file_dir/${slug}.stdout"
  local err="$per_file_dir/${slug}.stderr"

  if ! PGPASSWORD= psql -X -h "$host" -p "$port" -U "$user" "$db" \
    -c "CREATE DATABASE \"$dbname\"" >/dev/null 2>&1; then
    printf 'WARNING: CREATE DATABASE %s failed, skipping %s\n' "$dbname" "$rel" >&2
    return
  fi

  if (
    cd "$regress"
    timeout "${REGRESS_FILE_TIMEOUT:-30}" \
      psql -X -h "$host" -p "$port" -U "$user" "$dbname" \
        -v abs_srcdir="$regress" \
        -c '\set ON_ERROR_STOP off' \
        -c '\pset pager off' \
        -c '\timing off' \
        -f "$rel"
  ) >"$out" 2>"$err"; then
    : # success
  fi

  PGPASSWORD= psql -X -h "$host" -p "$port" -U "$user" "$db" \
    -c "DROP DATABASE IF EXISTS \"$dbname\"" >/dev/null 2>&1 \
    || printf 'WARNING: DROP DATABASE %s failed\n' "$dbname" >&2
}
export -f run_one_file
export regress host port user db per_file_dir PG_ABS_SRCDIR PG_LIBDIR PG_DLSUFFIX

# Always run test_setup.sql first to populate the template database, if it exists
if [[ -f "$sqldir/test_setup.sql" ]]; then
  setup_db="regress_sql_test_setup_sql"
  # Try to create the database. If it fails, it means another batch is already initializing the template.
  if ! PGPASSWORD= psql -X -h "$host" -p "$port" -U "$user" "$db" \
    -c "CREATE DATABASE \"$setup_db\"" >/dev/null 2>&1; then
    printf 'Another batch is initializing the template. Waiting for it to complete...\n'
    while PGPASSWORD= psql -X -h "$host" -p "$port" -U "$user" "$setup_db" -c "SELECT 1" >/dev/null 2>&1; do
      sleep 1
    done
    printf 'Template initialization complete.\n'
  else
    # We successfully created it, so we run the file and drop it
    printf 'Initializing template schema by running test_setup.sql...\n'
    setup_slug="sql_test_setup_sql"
    setup_out="$per_file_dir/${setup_slug}.stdout"
    setup_err="$per_file_dir/${setup_slug}.stderr"
    if (
      cd "$regress"
      timeout "${REGRESS_FILE_TIMEOUT:-30}" \
        psql -X -h "$host" -p "$port" -U "$user" "$setup_db" \
          -v abs_srcdir="$regress" \
          -c '\set ON_ERROR_STOP off' \
          -c '\pset pager off' \
          -c '\timing off' \
          -f "sql/test_setup.sql"
    ) >"$setup_out" 2>"$setup_err"; then
      :
    fi
    PGPASSWORD= psql -X -h "$host" -p "$port" -U "$user" "$db" \
      -c "DROP DATABASE IF EXISTS \"$setup_db\"" >/dev/null 2>&1 \
      || printf 'WARNING: DROP DATABASE %s failed\n' "$setup_db" >&2
  fi
fi

# Filter out test_setup.sql from the parallel run list
filtered_files=()
for f in "${sql_files[@]}"; do
  if [[ "$f" != *"/test_setup.sql" ]]; then
    filtered_files+=("$f")
  fi
done
sql_files=("${filtered_files[@]}")

printf 'Running %d SQL files with parallelism=%s\n' "${#sql_files[@]}" "$parallelism"

if [[ "${#sql_files[@]}" -gt 0 ]]; then
  printf '%s\n' "${sql_files[@]}" | \
    xargs -P "$parallelism" -I{} bash -c 'run_one_file "$@"' _ {}
fi

success_files=0
failed_files=0
completed_files=0
total_errors=0
total_fatals=0

printf 'status\tfile\terror_count\tcompleted\n' >"$file_results"
for file in "${sql_files[@]}"; do
  rel="${file#"$regress"/}"
  slug="$(printf '%s' "$rel" | tr '/.' '__')"
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
