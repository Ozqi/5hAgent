#!/usr/bin/env bash
set -euo pipefail

# 5hAgent 一键安装脚本
# 用法: curl -fsSL https://raw.githubusercontent.com/Ozqi/5hAgent/master/install.sh | bash
# 或者: bash install.sh

REPO="Ozqi/5hAgent"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.5hAgent"
BINARY_NAME="5hagent"

info()  { printf "\033[1;34m▸ %s\033[0m\n" "$*"; }
ok()    { printf "\033[1;32m✔ %s\033[0m\n" "$*"; }
warn()  { printf "\033[1;33m⚠ %s\033[0m\n" "$*"; }
err()   { printf "\033[1;31m✘ %s\033[0m\n" "$*" >&2; exit 1; }

# ── 1. 检查 Go ──
command -v go >/dev/null 2>&1 || err "未检测到 Go，请先安装 Go >= 1.24.2: https://go.dev/dl/"

GO_VERSION=$(go version | grep -oP 'go\K[0-9.]+')
GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 24 ]; }; then
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

# ── 3. 克隆或更新仓库 ──
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

info "克隆仓库..."
git clone --depth=1 "https://github.com/${REPO}.git" "$TMP_DIR/5hAgent" 2>/dev/null || \
git clone --depth=1 "git@github.com:${REPO}.git" "$TMP_DIR/5hAgent"

# ── 4. 编译 ──
info "编译 5hAgent..."
cd "$TMP_DIR/5hAgent"
go build -o "$BINARY_NAME" cmd/5hagent/main.go
ok "编译完成"

# ── 5. 安装二进制 ──
mkdir -p "$INSTALL_DIR"
cp "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"
ok "已安装到 $INSTALL_DIR/$BINARY_NAME"

# ── 6. 初始化配置目录 ──
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG_DIR/.env" ]; then
    cat > "$CONFIG_DIR/.env" << 'ENV'
# 5hAgent LLM 配置
LLM_API_KEY=your_api_key_here
LLM_BASE_URL=https://api.anthropic.com
LLM_MODEL=claude-sonnet-4-6
LLM_MAX_TOKENS=4096
AGENT_NAME=5hAgent
AGENT_MAX_TOTAL_TOKENS=200000
AGENT_REPEAT_TOOL_LIMIT=5
ENV
    ok "已创建默认配置: $CONFIG_DIR/.env"
else
    warn "配置文件已存在，跳过: $CONFIG_DIR/.env"
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

# ── 7. PATH 提示 ──
if echo "$PATH" | grep -q "$INSTALL_DIR"; then
    ok "$INSTALL_DIR 已在 PATH 中"
else
    warn "请将 $INSTALL_DIR 加入 PATH:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

# ── 8. 完成 ──
echo ""
ok "安装完成！运行方式:"
echo "  $INSTALL_DIR/$BINARY_NAME"
echo ""
echo "首次使用前请编辑配置文件，填入你的 LLM API Key:"
echo "  $CONFIG_DIR/.env"
