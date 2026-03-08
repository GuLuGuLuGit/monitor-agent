#!/bin/bash
# 安装 monitor-agent（示例：复制二进制与配置到指定目录）
set -e
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-/etc/monitor-agent}"
BINARY="agent"

if [ ! -f "./${BINARY}" ]; then
  echo "请先编译: go build -o agent ./cmd/agent"
  exit 1
fi

sudo mkdir -p "$CONFIG_DIR"
sudo cp "./${BINARY}" "$INSTALL_DIR/monitor-agent"
sudo chmod 755 "$INSTALL_DIR/monitor-agent"
[ -f ./config/config.yaml ] && sudo cp ./config/config.yaml "$CONFIG_DIR/" || true
echo "已安装到 $INSTALL_DIR/monitor-agent，配置目录 $CONFIG_DIR"
echo "运行: $INSTALL_DIR/monitor-agent -config=$CONFIG_DIR/config.yaml"
