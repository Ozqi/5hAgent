#!/bin/bash
# SWE-bench 自动化测试脚本
# 用于测试 5hAgent 在真实代码修复任务上的表现

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE="$SCRIPT_DIR/workspace"
AGENT="$SCRIPT_DIR/5hagent"
RESULTS_DIR="$SCRIPT_DIR/test_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 加载环境变量
if [ -f "$SCRIPT_DIR/.env" ]; then
    export $(grep -v '^#' "$SCRIPT_DIR/.env" | xargs)
fi

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 创建结果目录
mkdir -p "$RESULTS_DIR"

# 日志函数
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# 任务定义
declare -A TASKS
TASKS["task1_dir"]="astropy"
TASKS["task1_id"]="astropy__astropy-14365"
TASKS["task1_desc"]="QDP 命令大小写敏感问题"
TASKS["task1_file"]="astropy/io/ascii/qdp.py"
TASKS["task1_prompt"]="修复 astropy/io/ascii/qdp.py 中的 bug：在 _line_type 函数中，找到 re.compile(_type_re) 这一行，添加 re.IGNORECASE 标志，使正则表达式不区分大小写。这样小写的 QDP 命令（如 'read serr'）也能被识别。"

TASKS["task2_dir"]="astropy-6938"
TASKS["task2_id"]="astropy__astropy-6938"
TASKS["task2_desc"]="FITS D 指数替换问题"
TASKS["task2_file"]="astropy/io/fits/fitsrec.py"
TASKS["task2_prompt"]="修复 astropy/io/fits/fitsrec.py 中的 bug：找到包含 'Replace exponent separator' 注释的代码段，将 output_field.replace(encode_ascii('E'), encode_ascii('D')) 修改为 output_field = output_field.replace(encode_ascii('E'), encode_ascii('D'))，因为 replace 不是原地操作。"

TASKS["task3_dir"]="astropy-14182"
TASKS["task3_id"]="astropy__astropy-14182"
TASKS["task3_desc"]="RST 输出支持 header_rows"
TASKS["task3_file"]="astropy/io/ascii/rst.py"
TASKS["task3_prompt"]="为 astropy/io/ascii/rst.py 添加 header_rows 参数支持：1) 在 RST 类的 __init__ 方法中添加 header_rows 参数；2) 在 write 方法中处理 header_rows，参考 FixedWidth 类的实现；3) 确保能正确读取和写入带有多行表头的 RST 表格。"

# 恢复任务到初始状态
reset_task() {
    local task_dir=$1
    log_info "恢复 $task_dir 到初始状态..."
    cd "$WORKSPACE/$task_dir"
    git restore . 2>/dev/null || true
    git clean -fd 2>/dev/null || true
    log_success "已恢复"
}

# 运行单个任务测试
run_task() {
    local task_num=$1
    local task_dir="${TASKS[task${task_num}_dir]}"
    local task_id="${TASKS[task${task_num}_id]}"
    local task_desc="${TASKS[task${task_num}_desc]}"
    local task_file="${TASKS[task${task_num}_file]}"
    local task_prompt="${TASKS[task${task_num}_prompt]}"

    local result_file="$RESULTS_DIR/${task_id}_${TIMESTAMP}.log"

    echo ""
    echo "=========================================="
    log_info "任务 $task_num: $task_desc"
    log_info "ID: $task_id"
    log_info "目录: $WORKSPACE/$task_dir"
    echo "=========================================="

    # 恢复初始状态
    reset_task "$task_dir"

    # 记录开始时间
    local start_time=$(date +%s)

    # 运行 agent
    cd "$WORKSPACE/$task_dir"
    log_info "启动 5hAgent..."
    echo "$task_prompt" | timeout 300 "$AGENT" > "$result_file" 2>&1 || {
        log_error "Agent 执行失败或超时"
        echo "FAILED" > "$result_file.status"
        return 1
    }

    # 记录结束时间
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    # 检查是否有修改
    if git diff --quiet "$task_file"; then
        log_error "未检测到文件修改"
        echo "NO_CHANGES" > "$result_file.status"
        return 1
    fi

    # 保存 diff
    git diff "$task_file" > "$result_file.diff"

    log_success "任务完成，耗时 ${duration}s"
    log_info "结果保存到: $result_file"
    log_info "Diff 保存到: $result_file.diff"

    # 显示修改摘要
    echo ""
    log_info "修改摘要:"
    git diff --stat "$task_file"

    echo "SUCCESS:${duration}s" > "$result_file.status"
    return 0
}

# 生成测试报告
generate_report() {
    local report_file="$RESULTS_DIR/report_${TIMESTAMP}.md"

    log_info "生成测试报告..."

    cat > "$report_file" << EOF
# 5hAgent SWE-bench 测试报告

**测试时间**: $(date)
**Agent 版本**: $(cd "$SCRIPT_DIR" && git rev-parse --short HEAD 2>/dev/null || echo "unknown")

## 测试结果

EOF

    for i in 1 2 3; do
        local task_id="${TASKS[task${i}_id]}"
        local task_desc="${TASKS[task${i}_desc]}"
        local status_file="$RESULTS_DIR/${task_id}_${TIMESTAMP}.log.status"

        echo "### 任务 $i: $task_desc" >> "$report_file"
        echo "" >> "$report_file"
        echo "- **ID**: $task_id" >> "$report_file"

        if [ -f "$status_file" ]; then
            local status=$(cat "$status_file")
            if [[ $status == SUCCESS* ]]; then
                local duration=$(echo "$status" | cut -d: -f2)
                echo "- **状态**: ✅ 成功" >> "$report_file"
                echo "- **耗时**: $duration" >> "$report_file"
            else
                echo "- **状态**: ❌ 失败 ($status)" >> "$report_file"
            fi
        else
            echo "- **状态**: ⏭️ 未运行" >> "$report_file"
        fi

        echo "" >> "$report_file"
    done

    log_success "报告已生成: $report_file"
}

# 主函数
main() {
    echo "=========================================="
    echo "5hAgent SWE-bench 自动化测试"
    echo "=========================================="

    # 检查 agent 是否存在
    if [ ! -f "$AGENT" ]; then
        log_error "找不到 5hagent: $AGENT"
        exit 1
    fi

    # 选择测试模式
    if [ $# -eq 0 ]; then
        echo ""
        echo "用法:"
        echo "  $0 <task_number>  # 测试单个任务 (1-3)"
        echo "  $0 all            # 测试所有任务"
        echo ""
        exit 1
    fi

    if [ "$1" == "all" ]; then
        for i in 1 2 3; do
            run_task $i || log_warn "任务 $i 失败"
        done
        generate_report
    elif [[ "$1" =~ ^[1-3]$ ]]; then
        run_task "$1"
    else
        log_error "无效的任务编号: $1"
        exit 1
    fi

    echo ""
    echo "=========================================="
    log_success "测试完成"
    echo "=========================================="
}

main "$@"
