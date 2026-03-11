#!/bin/bash
#
# OpenClaw Monitor Agent 本地开发一键启动
#
# 用法:
#   ./dev-start.sh                          # 连接本地后端 (localhost:8080)
#   ./dev-start.sh --server http://IP:8010  # 连接远程服务器
#   ./dev-start.sh --info                   # 仅显示节点身份信息
#
set -e

PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_ROOT"

# 颜色
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${BLUE}[INFO]${NC}  $1"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
err()   { echo -e "${RED}[ERR]${NC}   $1"; exit 1; }

# ============================================================
# 默认值
# ============================================================
SERVER_URL="http://localhost:8080"
MQTT_BROKER="tcp://localhost:1883"
LOG_LEVEL="DEBUG"
SHOW_INFO=false

# ============================================================
# 解析参数
# ============================================================
while [[ $# -gt 0 ]]; do
    case "$1" in
        --server)     SERVER_URL="$2"; shift 2 ;;
        --mqtt)       MQTT_BROKER="$2"; shift 2 ;;
        --log-level)  LOG_LEVEL="$2"; shift 2 ;;
        --info)       SHOW_INFO=true; shift ;;
        -h|--help)
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --server URL     后端地址 (默认: http://localhost:8080)"
            echo "  --mqtt URL       MQTT Broker (默认: tcp://localhost:1883)"
            echo "  --log-level LVL  日志级别 DEBUG|INFO|WARN|ERROR (默认: DEBUG)"
            echo "  --info           仅显示节点身份信息"
            echo "  -h, --help       显示帮助"
            exit 0
            ;;
        *) err "未知参数: $1" ;;
    esac
done

# ============================================================
# 前置检查
# ============================================================
command -v go >/dev/null 2>&1 || err "未安装 Go，请先安装: https://go.dev/dl/"

if [ "$SHOW_INFO" = true ]; then
    go run ./cmd/agent/ --info
    exit 0
fi

# ============================================================
# 生成临时配置文件
# ============================================================
CONFIG_FILE="$PROJECT_ROOT/config/config.yaml"
mkdir -p "$PROJECT_ROOT/config"

info "生成配置文件..."
cat > "$CONFIG_FILE" <<EOF
server:
  url: "${SERVER_URL}"
  timeout: 30

device:
  data_dir: ""
  id_file: ""
  api_key_file: ""

transport:
  type: mqtt
  mqtt:
    broker: "${MQTT_BROKER}"
    keep_alive: 60
    clean_session: false
    auto_reconnect: true
    reconnect_interval: 5

redis:
  addr: "localhost:6379"
  password: ""
  db: 0

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
  level: "${LOG_LEVEL}"
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
EOF

ok "配置已生成: $CONFIG_FILE"

# ============================================================
# 显示启动信息
# ============================================================
echo ""
echo -e "${GREEN}══════════════════════════════════════════════${NC}"
echo -e "${GREEN}  OpenClaw Monitor Agent 启动中${NC}"
echo -e "${GREEN}══════════════════════════════════════════════${NC}"
echo ""
echo -e "  ${CYAN}后端${NC}   → ${SERVER_URL}"
echo -e "  ${CYAN}MQTT${NC}   → ${MQTT_BROKER}"
echo -e "  ${CYAN}日志${NC}   → ${LOG_LEVEL}"
echo -e "  ${CYAN}数据${NC}   → ~/.openclaw/"
echo ""
echo -e "${YELLOW}按 Ctrl+C 停止 Agent${NC}"
echo ""

# ============================================================
# 启动 Agent
# ============================================================
exec go run ./cmd/agent/ -config="$CONFIG_FILE"
