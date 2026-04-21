#!/bin/bash
# SWE-bench 测试运行器 - 3个新任务

set -e

TESTSPACE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ORIGINAL_DIR="$TESTSPACE_DIR/swebench_original"
AGENT="$(dirname "$TESTSPACE_DIR")/5hagent"
RESULTS_DIR="$TESTSPACE_DIR/results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 加载环境变量
if [ -f "$(dirname "$TESTSPACE_DIR")/.env" ]; then
    export $(grep -v '^#' "$(dirname "$TESTSPACE_DIR")/.env" | xargs)
fi

mkdir -p "$RESULTS_DIR"

# 任务定义 - 3个新任务
declare -A TASKS

# 任务 1: 字符串处理 - slugify 多空格问题
TASKS["1_id"]="task1-slugify"
TASKS["1_dir"]="task1"
TASKS["1_file"]="utils/text.py"
TASKS["1_problem"]="Fix slugify function to handle consecutive spaces correctly. Multiple consecutive spaces should be converted to a single dash, not multiple dashes. For example, 'hello  world' should become 'hello-world', not 'hello--world'."

# 任务 2: HTTP 请求 - timeout 参数未使用
TASKS["2_id"]="task2-timeout"
TASKS["2_dir"]="task2"
TASKS["2_file"]="http/client.py"
TASKS["2_problem"]="Fix make_request function to actually use the timeout parameter. The timeout parameter is accepted but not used in the response. Add 'timeout': timeout to the response dictionary."

# 任务 3: 列表工具 - 去重保持顺序
TASKS["3_id"]="task3-dedup"
TASKS["3_dir"]="task3"
TASKS["3_file"]="utils/lists.py"
TASKS["3_problem"]="Fix remove_duplicates function to preserve the order of first occurrence. Currently using set() which loses order. Use a different approach that maintains the original order while removing duplicates."

# 运行单个任务
run_task() {
    local task_num=$1
    local task_id="${TASKS[${task_num}_id]}"
    local task_dir="${TASKS[${task_num}_dir]}"
    local task_file="${TASKS[${task_num}_file]}"
    local task_problem="${TASKS[${task_num}_problem]}"

    local workspace="$TESTSPACE_DIR/${task_id}-workspace"
    local result_file="$RESULTS_DIR/${task_id}_${TIMESTAMP}.json"

    echo "=========================================="
    echo "任务 $task_num: $task_id"
    echo "=========================================="

    # 检查原始目录是否存在
    if [ ! -d "$ORIGINAL_DIR/$task_dir" ]; then
        echo "✗ 跳过 - 原始代码不存在: $ORIGINAL_DIR/$task_dir"
        cat > "$result_file" << EOF
{
  "task_id": "$task_id",
  "status": "SKIPPED",
  "duration_seconds": 0,
  "changes_lines": 0,
  "exit_code": 1,
  "workspace": "$workspace",
  "timestamp": "$TIMESTAMP",
  "error": "Original code not found"
}
EOF
        return 1
    fi

    # 清理并创建 workspace
    rm -rf "$workspace"
    cp -r "$ORIGINAL_DIR/$task_dir" "$workspace"
    cd "$workspace"
    git restore . 2>/dev/null || true
    git clean -fd 2>/dev/null || true

    echo "工作目录: $workspace"

    # 记录开始时间
    local start_time=$(date +%s)

    # 运行 agent
    echo "启动 5hAgent..."
    echo "$task_problem" | timeout 300 "$AGENT" > "$workspace/agent.log" 2>&1
    local exit_code=$?

    # 记录结束时间
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    # 检查结果
    local status="FAILED"
    local changes=0

    if [ $exit_code -eq 0 ] || [ $exit_code -eq 124 ]; then
        if ! git diff --quiet "$task_file" 2>/dev/null; then
            status="SUCCESS"
            changes=$(git diff --numstat "$task_file" | awk '{print $1+$2}')
            git diff "$task_file" > "$workspace/changes.diff"
            echo "✓ 成功 (${duration}s, ${changes} 行修改)"
        else
            status="NO_CHANGES"
            echo "✗ 未检测到修改 (${duration}s)"
        fi
    else
        status="ERROR"
        echo "✗ 执行失败 (exit code: $exit_code)"
    fi

    # 生成 JSON 报告
    cat > "$result_file" << EOF
{
  "task_id": "$task_id",
  "status": "$status",
  "duration_seconds": $duration,
  "changes_lines": $changes,
  "exit_code": $exit_code,
  "workspace": "$workspace",
  "timestamp": "$TIMESTAMP"
}
EOF

    echo "结果: $result_file"
    echo ""

    return $([ "$status" = "SUCCESS" ] && echo 0 || echo 1)
}

# 主函数
main() {
    echo "=========================================="
    echo "SWE-bench 测试 - 3个新任务"
    echo "=========================================="
    echo ""

    if [ $# -eq 0 ]; then
        echo "用法:"
        echo "  $0 <task_number>  # 测试单个任务 (1-3)"
        echo "  $0 all            # 测试所有任务"
        exit 1
    fi

    if [ "$1" = "all" ]; then
        for i in 1 2 3; do
            run_task $i || true
        done
    elif [[ "$1" =~ ^[1-3]$ ]]; then
        run_task "$1"
    else
        echo "无效的任务编号: $1"
        exit 1
    fi

    echo "=========================================="
    echo "测试完成"
    echo "=========================================="
}

main "$@"
