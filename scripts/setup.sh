#!/bin/bash
# ============================================================
#  Monitor Agent 一键部署脚本 (macOS)
#  用法: ./setup.sh
#  所有文件安装在用户目录下，无需 sudo 权限
# ============================================================
set -e

# ─── 颜色 ───────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ─── 配置（全部在用户目录下，无需 sudo） ────────────────────
AGENT_NAME="monitor-agent"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.${AGENT_NAME}"
DATA_DIR="${HOME}/.${AGENT_NAME}"
LAUNCHD_PLIST="${HOME}/Library/LaunchAgents/com.monitor.agent.plist"
REPO_DIR=""

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[✗]${NC} $1"; exit 1; }
step() { echo -e "\n${CYAN}${BOLD}>>> $1${NC}"; }

# ─── 检测脚本所在的项目目录 ─────────────────────────────────
detect_repo() {
  if [ -n "$AGENT_REPO_DIR" ]; then
    REPO_DIR="$AGENT_REPO_DIR"
  elif [ -f "./go.mod" ] && grep -q "monitor-agent" "./go.mod" 2>/dev/null; then
    REPO_DIR="$(pwd)"
  elif [ -f "$(dirname "$0")/../go.mod" ]; then
    REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
  else
    err "请在 monitor-agent 项目根目录下执行此脚本，或设置 AGENT_REPO_DIR 环境变量"
  fi
}

# ─── 检测已有安装 ───────────────────────────────────────────
EXISTING_INSTALL=false
KEEP_CONFIG=false

check_existing() {
  if [ -f "$CONFIG_DIR/config.yaml" ] && [ -f "$INSTALL_DIR/$AGENT_NAME" ]; then
    EXISTING_INSTALL=true
  fi
}

# ─── 确保 ~/.local/bin 在 PATH 中 ──────────────────────────
ensure_path() {
  if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    SHELL_RC=""
    if [ -f "$HOME/.zshrc" ]; then
      SHELL_RC="$HOME/.zshrc"
    elif [ -f "$HOME/.bashrc" ]; then
      SHELL_RC="$HOME/.bashrc"
    elif [ -f "$HOME/.bash_profile" ]; then
      SHELL_RC="$HOME/.bash_profile"
    fi

    if [ -n "$SHELL_RC" ]; then
      if ! grep -q "${INSTALL_DIR}" "$SHELL_RC" 2>/dev/null; then
        echo "" >> "$SHELL_RC"
        echo "# Monitor Agent" >> "$SHELL_RC"
        echo "export PATH=\"${INSTALL_DIR}:\$PATH\"" >> "$SHELL_RC"
        warn "已将 ${INSTALL_DIR} 添加到 ${SHELL_RC}，重启终端或执行: source ${SHELL_RC}"
      fi
    fi
    export PATH="${INSTALL_DIR}:$PATH"
  fi
}

# ─── 主流程 ─────────────────────────────────────────────────
echo ""
echo -e "${CYAN}${BOLD}╔══════════════════════════════════════╗${NC}"
echo -e "${CYAN}${BOLD}║   Monitor Agent 一键部署             ║${NC}"
echo -e "${CYAN}${BOLD}╚══════════════════════════════════════╝${NC}"
echo ""

check_existing

if [ "$EXISTING_INSTALL" = true ]; then
  echo -e "${YELLOW}${BOLD}检测到已有安装:${NC}"
  echo -e "  二进制: ${INSTALL_DIR}/${AGENT_NAME}"
  echo -e "  配置:   ${CONFIG_DIR}/config.yaml"
  CURRENT_URL=$(grep -A1 "^server:" "$CONFIG_DIR/config.yaml" 2>/dev/null | grep "url:" | sed 's/.*url: *"\?\([^"]*\)"\?/\1/' | tr -d ' ')
  if [ -n "$CURRENT_URL" ]; then
    echo -e "  后端:   ${CURRENT_URL}"
  fi
  echo ""
  echo -e "${YELLOW}请选择操作:${NC}"
  echo "  1) 保留现有配置，仅更新二进制（推荐用于代码更新）"
  echo "  2) 全新安装（重新输入所有配置）"
  echo ""
  read -p "请选择 [1/2] (默认 1): " CHOICE
  CHOICE="${CHOICE:-1}"

  if [ "$CHOICE" = "1" ]; then
    KEEP_CONFIG=true
    log "将保留现有配置，仅更新二进制文件"
  else
    log "将执行全新安装"
  fi
  echo ""
fi

# 1. 配置（仅全新安装时需要输入）
if [ "$KEEP_CONFIG" = false ]; then
  step "配置后端 API 地址"
  echo -e "请输入后端 API 地址（例如: ${BOLD}https://monitor.ikanban.cn${NC} 或 ${BOLD}http://1.2.3.4:8010${NC}）"
  read -p "> " SERVER_URL

  if [ -z "$SERVER_URL" ]; then
    err "API 地址不能为空"
  fi
  SERVER_URL="${SERVER_URL%/}"
  log "后端地址: ${SERVER_URL}"

  step "配置 MQTT Broker 地址（命令通道）"
  # 从后端地址自动推导默认 MQTT 地址
  MQTT_DEFAULT=""
  SERVER_HOST=$(echo "$SERVER_URL" | sed -E 's|https?://||' | sed 's|:[0-9]*$||' | sed 's|/.*||')
  if [ -n "$SERVER_HOST" ] && [ "$SERVER_HOST" != "localhost" ]; then
    MQTT_DEFAULT="tcp://${SERVER_HOST}:1883"
  else
    MQTT_DEFAULT="tcp://localhost:1883"
  fi
  echo -e "请输入 MQTT Broker 地址（默认: ${BOLD}${MQTT_DEFAULT}${NC}，直接回车使用默认值）"
  read -p "> " MQTT_BROKER
  MQTT_BROKER="${MQTT_BROKER:-$MQTT_DEFAULT}"
  log "MQTT 地址: ${MQTT_BROKER}"
fi

# 2. 检测项目目录
step "检测项目目录"
detect_repo
log "项目目录: ${REPO_DIR}"

# 3. 检查 Go 环境
step "检查 Go 编译环境"
if ! command -v go &>/dev/null; then
  err "未安装 Go，请先安装: https://go.dev/dl/"
fi
GO_VERSION=$(go version | awk '{print $3}')
log "Go 版本: ${GO_VERSION}"

# 4. 编译
step "编译 Agent"
cd "$REPO_DIR"
go build -o agent ./cmd/agent
log "编译完成: ${REPO_DIR}/agent"

# 5. 安装二进制（用户目录，无需 sudo）
step "安装二进制文件"
mkdir -p "$INSTALL_DIR"
cp agent "$INSTALL_DIR/$AGENT_NAME"
chmod 755 "$INSTALL_DIR/$AGENT_NAME"
log "已安装到 ${INSTALL_DIR}/${AGENT_NAME}"
ensure_path

# 6. 生成配置文件（仅全新安装时）
if [ "$KEEP_CONFIG" = false ]; then
  step "生成配置文件"
  mkdir -p "$CONFIG_DIR"
  cat > "$CONFIG_DIR/config.yaml" << YAML
server:
  url: "${SERVER_URL}"
  timeout: 30

transport:
  type: mqtt
  mqtt:
    broker: "${MQTT_BROKER}"
    keep_alive: 60
    clean_session: false
    auto_reconnect: true
    reconnect_interval: 5

intervals:
  heartbeat: 60
  metrics: 30
  skills: 300
  log_upload: 60

metrics:
  batch_size: 10

skills:
  scan_paths:
    - "~/.openclaw/skills"
    - "~/Library/Application Support/OpenClaw/skills"
    - "/usr/local/openclaw/skills"

logs:
  level: "INFO"
  file: ""
  max_size: 100
  max_backups: 3
  batch_size: 100

cache:
  dir: ""
  max_size_mb: 50

retry:
  max_attempts: 5
  initial_interval: 1
  max_interval: 30
YAML
  log "配置文件: ${CONFIG_DIR}/config.yaml"
else
  log "配置文件保持不变: ${CONFIG_DIR}/config.yaml"
fi

# 7. 创建数据目录
mkdir -p "$DATA_DIR"
log "数据目录: ${DATA_DIR}"

# 8. 安装 LaunchAgent（开机自启）
step "配置系统服务（LaunchAgent）"
mkdir -p "$(dirname "$LAUNCHD_PLIST")"
cat > "$LAUNCHD_PLIST" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.monitor.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${AGENT_NAME}</string>
        <string>-config=${CONFIG_DIR}/config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${DATA_DIR}/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>${DATA_DIR}/stderr.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>${INSTALL_DIR}:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
</dict>
</plist>
PLIST
log "LaunchAgent: ${LAUNCHD_PLIST}"

# 9. 启动服务
step "启动 Agent 服务"
launchctl unload "$LAUNCHD_PLIST" 2>/dev/null || true
launchctl load "$LAUNCHD_PLIST"
sleep 2

# 10. 验证
step "验证部署"
if launchctl list | grep -q "com.monitor.agent"; then
  PID=$(launchctl list | grep "com.monitor.agent" | awk '{print $1}')
  if [ "$PID" != "-" ] && [ -n "$PID" ]; then
    log "Agent 运行中 (PID: ${PID})"
  else
    warn "Agent 已加载但可能未启动，请查看日志"
  fi
else
  warn "Agent 未在 launchctl 中找到"
fi

sleep 1
if [ -f "${DATA_DIR}/agent.log" ]; then
  echo ""
  echo -e "${CYAN}最近日志:${NC}"
  tail -5 "${DATA_DIR}/agent.log" 2>/dev/null || true
elif [ -f "${DATA_DIR}/stderr.log" ]; then
  echo ""
  echo -e "${CYAN}最近日志:${NC}"
  tail -5 "${DATA_DIR}/stderr.log" 2>/dev/null || true
fi

# 完成
echo ""
echo -e "${GREEN}${BOLD}╔══════════════════════════════════════╗${NC}"
echo -e "${GREEN}${BOLD}║   部署完成!                          ║${NC}"
echo -e "${GREEN}${BOLD}╚══════════════════════════════════════╝${NC}"
echo ""
if [ "$KEEP_CONFIG" = true ] && [ -z "$SERVER_URL" ]; then
  SERVER_URL="$CURRENT_URL"
fi
echo -e "  二进制:   ${INSTALL_DIR}/${AGENT_NAME}"
echo -e "  配置文件: ${CONFIG_DIR}/config.yaml"
echo -e "  数据目录: ${DATA_DIR}"
echo -e "  服务:     ${LAUNCHD_PLIST}"
echo -e "  后端地址: ${SERVER_URL}"
echo ""
echo -e "${CYAN}常用命令:${NC}"
echo "  查看日志:   tail -f ${DATA_DIR}/agent.log"
echo "  重启服务:   launchctl unload ${LAUNCHD_PLIST} && launchctl load ${LAUNCHD_PLIST}"
echo "  停止服务:   launchctl unload ${LAUNCHD_PLIST}"
echo "  查看状态:   launchctl list | grep com.monitor.agent"
echo "  修改配置:   vim ${CONFIG_DIR}/config.yaml"
echo "  卸载:       ${REPO_DIR}/scripts/uninstall.sh"
echo ""
