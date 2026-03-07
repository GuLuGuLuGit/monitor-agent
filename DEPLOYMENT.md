# Monitor Agent - macOS 部署指南

## 📋 目录

1. [系统要求](#系统要求)
2. [快速部署](#快速部署)
3. [手动部署](#手动部署)
4. [服务管理](#服务管理)
5. [配置说明](#配置说明)
6. [故障排查](#故障排查)
7. [卸载](#卸载)

---

## 系统要求

- **操作系统**: macOS 10.15 (Catalina) 或更高版本
- **架构**: Intel (x86_64) 或 Apple Silicon (arm64)
- **磁盘空间**: 至少 100MB
- **内存**: 至少 50MB
- **网络**: 能够访问后端服务器

---

## 快速部署

### 1. 编译 Agent

```bash
cd monitor-agent

# Intel Mac
make build-darwin
# 生成: agent-darwin-amd64

# Apple Silicon Mac (M1/M2/M3)
GOOS=darwin GOARCH=arm64 go build -o agent-darwin-arm64 ./cmd/agent

# 或者直接编译当前架构
make build
```

### 2. 运行安装脚本

```bash
# 给安装脚本执行权限
chmod +x scripts/install.sh

# 运行安装
./scripts/install.sh
```

安装脚本会：
- ✅ 检查系统环境
- ✅ 安装二进制文件到 `/usr/local/bin`
- ✅ 创建配置文件
- ✅ 设置数据目录
- ✅ 创建 LaunchAgent（开机自启）
- ✅ 询问是否立即启动服务

### 3. 验证安装

```bash
# 检查服务状态
launchctl list | grep com.monitor.agent

# 查看日志
tail -f ~/.monitor-agent/agent.log

# 测试命令
monitor-agent -config=/usr/local/etc/monitor-agent/config.yaml
```

---

## 手动部署

如果不想使用安装脚本，可以手动部署：

### 1. 创建目录结构

```bash
# 创建必要的目录
sudo mkdir -p /usr/local/bin
sudo mkdir -p /usr/local/etc/monitor-agent
mkdir -p ~/.monitor-agent
mkdir -p ~/.monitor-agent/cache
```

### 2. 安装二进制文件

```bash
# 复制编译好的二进制文件
sudo cp agent /usr/local/bin/monitor-agent
sudo chmod +x /usr/local/bin/monitor-agent
```

### 3. 创建配置文件

```bash
# 创建配置文件
sudo tee /usr/local/etc/monitor-agent/config.yaml > /dev/null <<'EOF'
server:
  url: "http://your-backend-server:8080"  # 修改为你的后端地址
  timeout: 30

device:
  id_file: "~/.monitor-agent/device_id"
  api_key_file: "~/.monitor-agent/api_key"

intervals:
  heartbeat: 60
  metrics: 30
  skills: 300
  log_upload: 60

metrics:
  batch_size: 10

skills:
  scan_paths:
    - "~/Library/Application Support/OpenClaw/skills"
    - "/usr/local/openclaw/skills"

logs:
  level: "INFO"
  file: "~/.monitor-agent/agent.log"
  max_size: 100
  max_backups: 3
  batch_size: 100

cache:
  dir: "~/.monitor-agent/cache"
  max_size_mb: 50

retry:
  max_attempts: 5
  initial_interval: 1
  max_interval: 30
EOF
```

### 4. 创建 LaunchAgent

```bash
# 创建 LaunchAgent 配置
cat > ~/Library/LaunchAgents/com.monitor.agent.plist <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.monitor.agent</string>

    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/monitor-agent</string>
        <string>-config</string>
        <string>/usr/local/etc/monitor-agent/config.yaml</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>StandardOutPath</key>
    <string>/Users/YOUR_USERNAME/.monitor-agent/stdout.log</string>

    <key>StandardErrorPath</key>
    <string>/Users/YOUR_USERNAME/.monitor-agent/stderr.log</string>

    <key>WorkingDirectory</key>
    <string>/Users/YOUR_USERNAME/.monitor-agent</string>

    <key>ThrottleInterval</key>
    <integer>10</integer>
</dict>
</plist>
EOF

# 注意：将 YOUR_USERNAME 替换为你的用户名
```

### 5. 加载并启动服务

```bash
# 加载 LaunchAgent
launchctl load ~/Library/LaunchAgents/com.monitor.agent.plist

# 验证服务已启动
launchctl list | grep com.monitor.agent
```

---

## 服务管理

### 启动服务

```bash
launchctl load ~/Library/LaunchAgents/com.monitor.agent.plist
```

### 停止服务

```bash
launchctl unload ~/Library/LaunchAgents/com.monitor.agent.plist
```

### 重启服务

```bash
launchctl unload ~/Library/LaunchAgents/com.monitor.agent.plist
launchctl load ~/Library/LaunchAgents/com.monitor.agent.plist
```

### 查看服务状态

```bash
# 查看服务是否运行
launchctl list | grep com.monitor.agent

# 查看详细状态
launchctl print gui/$(id -u)/com.monitor.agent
```

### 查看日志

```bash
# 查看 Agent 日志
tail -f ~/.monitor-agent/agent.log

# 查看标准输出
tail -f ~/.monitor-agent/stdout.log

# 查看错误输出
tail -f ~/.monitor-agent/stderr.log

# 查看最近 100 行
tail -n 100 ~/.monitor-agent/agent.log
```

### 手动运行（调试用）

```bash
# 前台运行，查看输出
/usr/local/bin/monitor-agent -config=/usr/local/etc/monitor-agconfig.yaml

# 使用 Ctrl+C 停止
```

---

## 配置说明

### 修改配置

```bash
# 编辑配置文件
sudo nano /usr/local/etc/monitor-agent/config.yaml

# 或使用 vim
sudo vim /usr/local/etc/monitor-agent/config.yaml
```

### 重要配置项

#### 后端服务器地址
```yaml
server:
  url: "http://your-backend-server:8080"  # 必须修改
  timeout: 30
```

#### 采集间隔
```yaml
intervals:
  heartbeat: 60    # 心跳间隔（秒）
  metrics: 30      # 指标采集间隔（秒）
  skills: 300      # Skills 扫描间隔（秒）
  log_upload: 60   # 日志上报间隔（秒）
```

#### Skills 扫描路径
```yaml
skills:
  scan_paths:
    - "~/Library/Application Support/OpenClaw/skills"
    - "/usr/local/openclaw/skills"
    # 可以添加更多路径
```

#### 日志级别
```yaml
logs:
  level: "INFO"  # DEBUG, INFO, WARN, ERROR
```

### 配置修改后重启服务

```bash
launchctl unload ~/Library/LaunchAgents/com.monitor.agent.plist
launchctl load ~/Library/LaunchAgents/com.monitor.agent.plist
```

---

## 故障排查

### 1. 服务无法启动

**检查日志**:
```bash
tail -f ~/.monitor-agent/stderr.log
```

**常见原因**:
- 后端服务器地址配置错误
- 网络无法连接到后端
- 配置文件格式错误
- 权限问题

**解决方法**:
```bash
# 检查配置文件语法
cat /usr/local/etc/monitor-agent/config.yaml

# 测试网络连接
curl http://your-backend-server:8080/health

# 手动运行查看错误
/usr/local/bin/monitor-agent -config=/usr/local/etc/monitor-agent/config.yaml
```

### 2. 设备注册失败

**症状**: 日志显示 "register device failed"

**检查**:
```bash
# 查看日志
grep "register" ~/.monitor-agent/agent.log

# 检查后端服务是否正常
curl -X POST http://your-backend-server:8080/api/agent/device/register \
  -H "Content-Type: application/json" \
  -d '{"device_id":"test","hostname":"test","mac_address":"00:00:00:00:00:00"}'
```

**解决方法**:
- 确认后端服务正常运行
- 检查网络连接
- 查看后端日志

### 3. 指标上报失败

**症状**: 日志显示 "upload metrics failed"

**检查**:
```bash
# 查看详细错误
grep "upload.*faiitor-agent/agent.log

# 检查 API Key 是否存在
cat ~/.monitor-agent/api_key
```

**解决方法**:
- 删除 device_id 和 api_key，重新注册
```bash
rm ~/.monitor-agent/device_id
rm ~/.monitor-agent/api_key
launchctl unload ~/Library/LaunchAgents/com.monitor.agent.plist
launchctl load ~/Library/LaunchAgents/com.monitor.agent.plist
```

### 4. 权限问题

**症状**: 日志显示 "permission denied"

**解决方法**:
```bash
# 检查文件权限
ls -la ~/.monitor-agent/

# 修复权限
chmod 600 ~/.monitor-agent/api_key
chmod 600 ~/.monitor-agent/device_id
chmod 755 ~/.monitor-agent/
```

### 5. 查看系统日志

```bash
# 查看 launchd 日志
log show --predicate 'process == "launchd"' --last 1h | grep monitor

# 查看所有相关日志
log show --predicate 'eventMessage contains "monitor-agent"' --last 1h
```

---

## 卸载

### 使用卸载脚本

```bash
# 给卸载脚本执行权限
chmod +x scripts/uninstall.sh

# 运行卸载
./scripts/uninstall.sh
```

### 手动卸载

```bash
# 1. 停止并删除服务
launchctl unload ~/Library/LaunchAgents/com.monitor.agent.plist
rm ~/Library/LaunchAgents/com.monitor.agent.plist

# 2. 删除二进制文件
sudo rm /usr/local/bin/monitor-agent

# 3. 删除配置文件
sudo rm -rf /usr/local/etc/monitor-agent

# 4. 删除数据目录（可选，包含 device_id 和 api_key）
rm -rf ~/.monitor-agent
```

---

## 高级配置

### 1. 使用 HTTPS

修改配置文件中的后端yaml
server:
  url: "https://your-backend-server:443"
```

### 2. 自定义数据目录

```yaml
device:
  id_file: "/custom/path/device_id"
  api_key_file: "/custom/path/api_key"

logs:
  file: "/custom/path/agent.log"

cache:
  dir: "/custom/path/cache"
```

### 3. 调整资源限制

在 LaunchAgent 中添加：
```xml
<key>SoftResourceLimits</key>
<dict>
    <key>NumberOfFiles</key>
    <integer>1024</integer>
</dict>

<key>HardResourceLimits</key>
<dict>
    <key>NumberOfFiles</key>
    <integer>2048</integer>
</dict>
```

### 4. 设置环境变量

在 LaunchAgent 中添加：
```xml
<key>EnvironmentVariables</key>
<dict>
    <key>HTTP_PROXY</key>
    <string>http://proxy.example.com:8080</string>
    <key>HTTPS_PROXY</key>
    <string>http://proxy.example.com:8080</string>
</dict>
```

---

## 监控和维护

### 定期检查

```bash
# 检查服务状态
launchctl list | grep com.monitor.agent

# 检查日志大小
du -sh ~/.monitor-agent/

# 检查缓存大小
du -sh ~/.monitor-agent/cache/
```

### 日志轮转

Agent 自动进行日志轮转，配置：
```yaml
logs:
  max_size: 100      # 单个日志文件最大 100MB
  max_backups: 3     # 保留 3 个旧日志文件
```

### 清理缓存

```bash
# 清理缓存（服务会自动重建）
rm -rf ~/.monitor-agent/cache/*
```

---

##AQ)

### Q: Agent 占用多少资源？
A: 正常情况下：
- CPU: <1%
- 内存: ~50MB
- 磁盘: ~50MB（包含日志和缓存）

### Q: 如何更新 Agent？
A:
```bash
# 1. 停止服务
launchctl unload ~/Library/LaunchAgents/com.monitor.agent.plist

# 2. 替换二进制文件
sudo cp new-agent /usr/local/bin/monitor-agent

# 3. 启动服务
launchctl load ~/Library/LaunchAgents/com.monitor.agent.plist
```

### Q: 如何备份配置？
A:
```bash
# 备份配置和数据
tar -czf monitor-agent-backup.tar.gz \
  /usr/local/etc/monitor-agent \
  ~/.monitor-agent/device_id \
  ~/.monitor-agent/api_key
```

### Q: 支持多个后端服务器吗？
A: 当前版本不支持，但可以运行多个 Agent 实例（使用不同的配置文件和数据目录）。

---

## 技术支持

如遇问题，请提供以下信息：
1. macOS 版本: `sw_vers`
2. Agent 版本: `monitor-agent -version`
3. 错误日志: `~/.monitor-agent/agent.log`
4. 系统日志: `~/.monitor-agent/stderr.log`

---

**部署完成后，Agent 将自动开始采集和上报数据！** 🎉
