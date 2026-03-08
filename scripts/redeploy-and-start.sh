#!/usr/bin/env bash
# ============================================================
#  Agent 重新编译部署并重启服务
#  用法: ./redeploy-and-start.sh
# ============================================================
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
AGENT_NAME="monitor-agent"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.${AGENT_NAME}"
LAUNCHD_PLIST="${HOME}/Library/LaunchAgents/com.monitor.agent.plist"

echo -e "${CYAN}=== Agent 重新部署 ===${NC}"
echo ""

# 1. 编译
echo -e "${YELLOW}[1/4] 编译...${NC}"
cd "$AGENT_DIR"
go build -o agent ./cmd/agent
echo -e "${GREEN}[✓] 编译完成${NC}"

# 2. 停止服务
echo -e "${YELLOW}[2/4] 停止服务...${NC}"
launchctl unload "$LAUNCHD_PLIST" 2>/dev/null || true
echo -e "${GREEN}[✓] 服务已停止${NC}"

# 3. 安装
echo -e "${YELLOW}[3/4] 安装二进制...${NC}"
mkdir -p "$INSTALL_DIR"
cp agent "$INSTALL_DIR/$AGENT_NAME"
chmod 755 "$INSTALL_DIR/$AGENT_NAME"
echo -e "${GREEN}[✓] 已安装到 ${INSTALL_DIR}/${AGENT_NAME}${NC}"

# 4. 启动
echo -e "${YELLOW}[4/4] 启动服务...${NC}"
launchctl load "$LAUNCHD_PLIST"
sleep 2

if launchctl list | grep -q "com.monitor.agent"; then
  PID=$(launchctl list | grep "com.monitor.agent" | awk '{print $1}')
  echo -e "${GREEN}[✓] Agent 运行中 (PID: ${PID})${NC}"
else
  echo -e "${RED}[✗] 启动失败，请查看日志${NC}"
fi

echo ""
echo -e "${CYAN}查看日志: tail -f ~/.monitor-agent/agent.log${NC}"
