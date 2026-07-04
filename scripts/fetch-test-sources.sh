#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$root/external"

fetch_sparse() {
  local repo="$1"
  local dir="$2"
  local sparse="$3"
  if [[ -d "$dir/.git" ]]; then
    git -C "$dir" fetch --depth 1 origin master
    git -C "$dir" checkout --force FETCH_HEAD
    return
  fi
  git clone --depth 1 --filter=blob:none --sparse "$repo" "$dir"
  git -C "$dir" sparse-checkout set "$sparse"
}

fetch_full() {
  local repo="$1"
  local dir="$2"
  if [[ -d "$dir/.git" ]]; then
    git -C "$dir" fetch --depth 1 origin main || git -C "$dir" fetch --depth 1 origin master
    git -C "$dir" checkout --force FETCH_HEAD
    return
  fi
  git clone --depth 1 "$repo" "$dir"
}

fetch_sparse "https://github.com/postgres/postgres.git" "$root/external/postgres" "src/test/regress"
fetch_full "https://github.com/sqlancer/sqlancer.git" "$root/external/sqlancer"

printf 'Fetched PostgreSQL regress tests and SQLancer into %s/external\n' "$root"
