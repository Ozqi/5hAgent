#!/usr/bin/env bash
# systemd_l4_audit.sh - Agent Systemd L4 worklog 判定脚本。
# 调用方：真实 daemon 基准测试后手动或 CI 调用。
# 全局状态：无；只读取传入的 worklog 文件。

set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/systemd_l4_audit.sh --mode no-tool <worklog.md>...
  scripts/systemd_l4_audit.sh --mode require-tool <worklog.md>...

Modes:
  no-tool       worklog 中不得出现 Tool Event
  require-tool  worklog 中必须出现 Tool Event
USAGE
}

mode=""
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -ge 2 && "${1:-}" == "--mode" ]]; then
  mode="$2"
  shift 2
fi

if [[ -z "$mode" || $# -eq 0 ]]; then
  usage >&2
  exit 2
fi

case "$mode" in
  no-tool|require-tool) ;;
  *)
    echo "invalid mode: $mode" >&2
    usage >&2
    exit 2
    ;;
esac

status=0
for file in "$@"; do
  if [[ ! -f "$file" ]]; then
    echo "missing worklog: $file" >&2
    status=1
    continue
  fi

  if grep -q '^## Tool Event$' "$file"; then
    has_tool=1
  else
    has_tool=0
  fi

  case "$mode" in
    no-tool)
      if [[ "$has_tool" -eq 1 ]]; then
        echo "FAIL no-tool: tool event found in $file" >&2
        status=1
      else
        echo "PASS no-tool: $file"
      fi
      ;;
    require-tool)
      if [[ "$has_tool" -eq 0 ]]; then
        echo "FAIL require-tool: no tool event in $file" >&2
        status=1
      else
        echo "PASS require-tool: $file"
      fi
      ;;
  esac
done

exit "$status"
