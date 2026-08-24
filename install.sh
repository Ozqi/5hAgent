#!/usr/bin/env bash
set -euo pipefail

# walle 一键安装脚本
# 用法: curl -fsSL https://raw.githubusercontent.com/Ozqi/walle/master/install.sh | bash
# 或者: bash install.sh

REPO="Ozqi/walle"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.walle"
BINARY_NAME="walle"

info()  { printf "\033[1;34m▸ %s\033[0m\n" "$*"; }
ok()    { printf "\033[1;32m✔ %s\033[0m\n" "$*"; }
warn()  { printf "\033[1;33m⚠ %s\033[0m\n" "$*"; }
err()   { printf "\033[1;31m✘ %s\033[0m\n" "$*" >&2; exit 1; }

env_has_key() {
    local file="$1"
    local key="$2"
    grep -q "^[[:space:]]*$key=" "$file"
}

env_value() {
    local file="$1"
    local key="$2"
    (grep "^[[:space:]]*$key=" "$file" || true) | tail -1 | sed "s/^[[:space:]]*$key=//" | sed 's/^["'\'']//; s/["'\'']$//'
}

append_if_missing() {
    local file="$1"
    local key="$2"
    local value="$3"
    if [ -n "$value" ] && ! env_has_key "$file" "$key"; then
        printf "%s=%s\n" "$key" "$value" >> "$file"
    fi
}

install_skill() {
    local name="$1"
    local source="$SRC_DIR/skills/$name"
    local target="$CONFIG_DIR/skills/$name"
    if [ -e "$target" ] || [ -L "$target" ]; then
        warn "Skill 已存在，保留用户版本: $target"
        return
    fi
    mkdir -p "$CONFIG_DIR/skills"
    cp -R "$source" "$target"
    ok "已预装 Skill: $name"
}

daemon_pids() {
    {
        pgrep -f "(^|[ /])${BINARY_NAME} daemon" 2>/dev/null || true
        if command -v lsof >/dev/null 2>&1 && [ -e "${INSTALL_DIR}/${BINARY_NAME}" ]; then
            lsof -t "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null || true
        fi
    } | sort -u
}

stop_daemon_for_install() {
    DAEMON_WAS_RUNNING=0
    local pids
    pids=$(daemon_pids)
    if [ -z "$pids" ]; then
        return
    fi
    DAEMON_WAS_RUNNING=1
    warn "检测到正在运行的 walle daemon，安装前先停止: $pids"
    # shellcheck disable=SC2086
    kill $pids 2>/dev/null || true
    for _ in 1 2 3 4 5 6 7 8 9 10; do
        [ -z "$(daemon_pids)" ] && return
        sleep 0.2
    done
    warn "daemon 未及时退出，请手动检查: $(daemon_pids)"
}

restart_daemon_after_install() {
    if [ "${DAEMON_WAS_RUNNING:-0}" != "1" ]; then
        return
    fi
    info "重启 walle daemon..."
    nohup "${INSTALL_DIR}/${BINARY_NAME}" daemon >/dev/null 2>&1 &
    ok "daemon 已重启"
}

migrate_legacy_env() {
    local file="$1"
    if ! env_has_key "$file" "LLM_PROVIDER" && ! env_has_key "$file" "LLM_SUPPLIER"; then
        return
    fi
    cp "$file" "$file.bak.$(date +%Y%m%d%H%M%S)"
    if ! env_has_key "$file" "LLM_MODEL"; then
        local provider
        provider=$(env_value "$file" "LLM_SUPPLIER")
        if [ -z "$provider" ]; then
            provider=$(env_value "$file" "LLM_PROVIDER")
        fi
        provider=$(printf "%s" "$provider" | tr '[:upper:]' '[:lower:]')
        local upper
        upper=$(printf "%s" "$provider" | tr '[:lower:]' '[:upper:]')
        local model
        model=$(env_value "$file" "LLM_${upper}_MODEL")
        if [ -z "$model" ] && [ "$provider" = "openai" ]; then
            model=$(env_value "$file" "LLM_OPENAI_MODEL")
        fi
        if [ -n "$provider" ] && [ -n "$model" ]; then
            append_if_missing "$file" "LLM_MODEL" "$provider/$model"
            ok "已从旧配置补充 LLM_MODEL=$provider/$model"
        else
            warn "检测到旧 LLM_PROVIDER/LLM_SUPPLIER 配置，已备份；请按 LLM_MODEL=provider/model 格式整理: $file"
        fi
    else
        warn "检测到旧 LLM_PROVIDER/LLM_SUPPLIER 配置，已备份；当前 LLM_MODEL 已存在，未自动改写。"
    fi
}


# ── 1. 检查 Go ──
command -v go >/dev/null 2>&1 || err "未检测到 Go，请先安装 Go >= 1.24.2: https://go.dev/dl/"

GO_VERSION=$(go env GOVERSION 2>/dev/null || go version | awk '{print $3}')
GO_VERSION=${GO_VERSION#go}
IFS=. read -r GO_MAJOR GO_MINOR GO_PATCH <<EOF
$GO_VERSION
EOF
GO_MAJOR=${GO_MAJOR:-0}
GO_MINOR=${GO_MINOR:-0}
GO_PATCH=${GO_PATCH:-0}
if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 24 ]; } || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -eq 24 ] && [ "$GO_PATCH" -lt 2 ]; }; then
    err "Go 版本 $GO_VERSION 过低，需要 >= 1.24.2"
fi
ok "Go $GO_VERSION"

# ── 2. 检查 Node.js (MCP 依赖) ──
if command -v node >/dev/null 2>&1; then
    NODE_VERSION=$(node -v)
    ok "Node.js $NODE_VERSION"
else
    warn "未检测到 Node.js，MCP 功能将不可用（可选安装: https://nodejs.org/）"
fi

# ── 3. 定位源码 ──
TMP_DIR=""
if [ -f "go.mod" ] && [ -d "cmd/walle" ]; then
    SRC_DIR=$(pwd)
    info "使用当前源码目录: $SRC_DIR"
else
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "$TMP_DIR"' EXIT

    info "克隆仓库..."
    git clone --depth=1 "https://github.com/${REPO}.git" "$TMP_DIR/walle" 2>/dev/null || \
    git clone --depth=1 "git@github.com:${REPO}.git" "$TMP_DIR/walle"
    SRC_DIR="$TMP_DIR/walle"
fi

# ── 4. 编译 ──
info "编译 walle..."
cd "$SRC_DIR"
go build -o "$BINARY_NAME" ./cmd/walle
ok "编译完成"

# ── 5. 安装二进制 ──
mkdir -p "$INSTALL_DIR"
stop_daemon_for_install
INSTALL_TMP="$INSTALL_DIR/.${BINARY_NAME}.$$"
rm -f "$INSTALL_TMP"
install -m 0755 "$BINARY_NAME" "$INSTALL_TMP"
mv -f "$INSTALL_TMP" "$INSTALL_DIR/$BINARY_NAME"
ok "已安装到 $INSTALL_DIR/$BINARY_NAME"

# ── 5.5. 安装 zsh 补全 ──
if command -v zsh >/dev/null 2>&1 && [ -d "${ZSH:-$HOME/.oh-my-zsh}" ]; then
    ZSH_COMPLETION_DIR="${ZSH_CUSTOM:-$HOME/.oh-my-zsh/custom}/completions"
    mkdir -p "$ZSH_COMPLETION_DIR"
    cp "$SRC_DIR/completions/_$BINARY_NAME" "$ZSH_COMPLETION_DIR/_$BINARY_NAME"
    ok "已安装 zsh Tab 补全到 $ZSH_COMPLETION_DIR/_$BINARY_NAME"
fi

# ── 6. 初始化配置目录 ──
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG_DIR/.env" ]; then
    cp "$SRC_DIR/.env.example" "$CONFIG_DIR/.env"
    ok "已创建默认配置: $CONFIG_DIR/.env"
    migrate_legacy_env "$CONFIG_DIR/.env"
else
    warn "配置文件已存在，跳过: $CONFIG_DIR/.env"
    migrate_legacy_env "$CONFIG_DIR/.env"
fi

if [ ! -f "$CONFIG_DIR/mcp.json" ]; then
    cat > "$CONFIG_DIR/mcp.json" << 'JSON'
{
  "servers": []
}
JSON
    ok "已创建 MCP 配置: $CONFIG_DIR/mcp.json"
else
    warn "MCP 配置已存在，跳过: $CONFIG_DIR/mcp.json"
fi

# ── 6.5. 安装 prompt 模板 ──
mkdir -p "$CONFIG_DIR/prompt"
cp -r "$SRC_DIR/prompt/"*.md "$CONFIG_DIR/prompt/" 2>/dev/null || true
ok "已安装 prompt 模板到 $CONFIG_DIR/prompt/"

# ── 6.6. 安装内置 Skill；已有同名目录视为用户版本，不覆盖 ──
install_skill "tmux-skill"
install_skill "agent-reach"

# ── 7. PATH 提示 ──
if echo "$PATH" | grep -q "$INSTALL_DIR"; then
    ok "$INSTALL_DIR 已在 PATH 中"
else
    warn "请将 $INSTALL_DIR 加入 PATH:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

# ── 8. 完成 ──
restart_daemon_after_install
echo ""
ok "安装完成！运行方式:"
echo "  $INSTALL_DIR/$BINARY_NAME"
echo ""
echo "默认配置使用本地 Ollama:"
echo "  LLM_MODEL=ollama/ornith:9b"
echo "  LLM_OLLAMA_BASE_URL=http://localhost:11434/v1"
echo ""
echo "如未安装默认模型，请执行:"
echo "  ollama pull ornith:9b"
echo ""
echo "如需切换远端模型，请编辑:"
echo "  $CONFIG_DIR/.env"
echo "配置文件内已包含 OpenAI-compatible、DeepSeek、OpenRouter、Claude 和 ChatGPT OAuth 示例。"
