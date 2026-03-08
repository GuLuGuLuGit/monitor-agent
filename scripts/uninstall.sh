#!/bin/bash
# Monitor Agent 卸载脚本 for macOS

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

AGENT_NAME="monitor-agent"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.${AGENT_NAME}"
DATA_DIR="${HOME}/.${AGENT_NAME}"
LAUNCHD_PLIST="${HOME}/Library/LaunchAgents/com.monitor.agent.plist"

echo -e "${YELLOW}=== Monitor Agent 卸载程序 ===${NC}"
echo ""
echo -e "${RED}警告: 此操作将删除 Agent 二进制和服务配置${NC}"
echo -e "${YELLOW}是否继续? (y/n)${NC}"
read -p "> " CONFIRM

if [[ "$CONFIRM" != "y" && "$CONFIRM" != "Y" ]]; then
    echo "取消卸载"
    exit 0
fi

echo ""

# 1. 停止服务
echo -e "${YELLOW}[1/4] 停止服务...${NC}"
if [ -f "${LAUNCHD_PLIST}" ]; then
    launchctl unload "${LAUNCHD_PLIST}" 2>/dev/null || true
    echo -e "${GREEN}✓ 服务已停止${NC}"
else
    echo -e "${YELLOW}! 服务配置文件不存在${NC}"
fi
echo ""

# 2. 删除 LaunchAgent
echo -e "${YELLOW}[2/4] 删除系统服务配置...${NC}"
if [ -f "${LAUNCHD_PLIST}" ]; then
    rm -f "${LAUNCHD_PLIST}"
    echo -e "${GREEN}✓ LaunchAgent 已删除${NC}"
else
    echo -e "${YELLOW}! LaunchAgent 不存在${NC}"
fi
echo ""

# 3. 删除二进制文件
echo -e "${YELLOW}[3/4] 删除二进制文件...${NC}"
if [ -f "${INSTALL_DIR}/${AGENT_NAME}" ]; then
    rm -f "${INSTALL_DIR}/${AGENT_NAME}"
    echo -e "${GREEN}✓ 二进制文件已删除${NC}"
else
    echo -e "${YELLOW}! 二进制文件不存在${NC}"
fi
echo ""

# 4. 询问是否删除数据和配置
echo -e "${YELLOW}[4/4] 处理数据目录...${NC}"
if [ -d "${DATA_DIR}" ]; then
    echo -e "${YELLOW}是否删除数据和配置目录? (包含 device_id, api_key, 配置文件, 日志和缓存)${NC}"
    echo "  ${DATA_DIR}"
    read -p "> (y/n): " DELETE_DATA

    if [[ "$DELETE_DATA" == "y" || "$DELETE_DATA" == "Y" ]]; then
        rm -rf "${DATA_DIR}"
        echo -e "${GREEN}✓ 数据目录已删除${NC}"
    else
        echo -e "${YELLOW}! 数据目录已保留${NC}"
        echo "  如需手动删除: rm -rf ${DATA_DIR}"
    fi
else
    echo -e "${YELLOW}! 数据目录不存在${NC}"
fi
echo ""

echo -e "${GREEN}=== 卸载完成 ===${NC}"
echo ""
echo "已删除:"
echo "  - 二进制文件: ${INSTALL_DIR}/${AGENT_NAME}"
echo "  - LaunchAgent: ${LAUNCHD_PLIST}"
if [[ "$DELETE_DATA" == "y" || "$DELETE_DATA" == "Y" ]]; then
    echo "  - 数据/配置目录: ${DATA_DIR}"
fi
echo ""
echo -e "${GREEN}卸载完成！${NC}"
