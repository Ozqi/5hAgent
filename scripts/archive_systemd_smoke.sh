#!/usr/bin/env bash
# archive_systemd_smoke.sh - 归档 5hWorkSpace 中的 Agent Systemd 冒烟测试产物。
# 调用方：真实 daemon 基准测试后手动执行；默认 dry-run。
# 全局状态：只在 --apply 时移动 workspace .5hagent 下的已知 smoke report/log。

set -euo pipefail

workspace="/Users/bytedance/Proj/5hWorkSpace"
apply=0

usage() {
  cat <<'USAGE'
Usage:
  scripts/archive_systemd_smoke.sh [--workspace <dir>] [--apply]

Default:
  Dry-run only. Prints files that would move.

Moves:
  <workspace>/.5hagent/reports/systemd-*.md
  <workspace>/.5hagent/agents/*/logs/*systemd-*.md

Destination:
  <workspace>/.5hagent/archive/systemd-smoke/<YYYYMMDD-HHMMSS>/
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --workspace)
      workspace="${2:-}"
      shift 2
      ;;
    --apply)
      apply=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$workspace" || ! -d "$workspace/.5hagent" ]]; then
  echo "workspace .5hagent not found: $workspace" >&2
  exit 1
fi

stamp="$(date -u +%Y%m%d-%H%M%S)"
dest="$workspace/.5hagent/archive/systemd-smoke/$stamp"

files=()
while IFS= read -r file; do
  files+=("$file")
done < <(
  {
    find "$workspace/.5hagent/reports" -maxdepth 1 -type f -name 'systemd-*.md' 2>/dev/null || true
    find "$workspace/.5hagent/agents" -type f -path '*/logs/*systemd-*.md' 2>/dev/null || true
  } | sort
)

if [[ "${#files[@]}" -eq 0 ]]; then
  echo "no systemd smoke artifacts found under $workspace/.5hagent"
  exit 0
fi

if [[ "$apply" -eq 0 ]]; then
  echo "dry-run: would move ${#files[@]} files to $dest"
  printf '%s\n' "${files[@]}"
  exit 0
fi

mkdir -p "$dest"
for file in "${files[@]}"; do
  rel="${file#"$workspace/.5hagent/"}"
  target="$dest/$rel"
  mkdir -p "$(dirname "$target")"
  mv "$file" "$target"
done

echo "moved ${#files[@]} files to $dest"
