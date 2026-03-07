# Monitor Agent（Mac 端采集 Agent）

根据 [MAC_AGENT_TASKS.md](../MAC_AGENT_TASKS.md) 实现的 Mac 端监控采集 Agent，用于收集系统指标、OpenClaw Skills 信息和日志，并上报到监控后端服务。

## 功能概览

- **设备注册与认证**：首次启动向后端注册，保存 API Key，后续请求携带 `X-API-Key`
- **心跳**：按配置间隔上报心跳保持在线
- **系统指标**：CPU/内存/磁盘/网络/负载/进程数，定时采集并批量上报
- **OpenClaw Skills**：扫描配置目录下的 Skills，定时上报
- **日志**：Agent 自身日志缓冲后批量上报
- **配置**：YAML 配置，支持 server、间隔、批次大小、Skills 路径等
- **重试与缓存**：请求失败指数退避重试；可选离线文件缓存

## 目录结构

```
monitor-agent/
├── cmd/agent/           # 主程序入口
├── internal/
│   ├── collector/       # 指标与 Skills 采集
│   ├── uploader/        # 注册、心跳、指标/Skills/日志上报
│   ├── config/          # 配置加载与校验
│   ├── device/          # 设备信息与注册
│   └── cache/           # 离线缓存
├── pkg/
│   ├── client/          # HTTP 客户端（重试、API Key）
│   └── logger/          # 日志与上报缓冲
├── config/              # 默认配置
├── scripts/             # 脚本
└── README.md
```

## 依赖

- Go 1.21+
- 后端 API：`/api/v1/agent`（设备注册、心跳、指标、Skills、日志）

## 配置

默认会从当前目录、`config/`、`/etc/monitor-agent` 查找 `config.yaml`，或通过 `-config=/path/to/config.yaml` 指定。

主要配置项见 `config/config.yaml`：

- `server.url`：后端地址（如 `http://localhost:8080`）
- `server.timeout`：请求超时（秒）
- `device.id_file` / `device.api_key_file`：不填则使用 `~/.monitor-agent/` 下文件
- `intervals.heartbeat` / `metrics` / `skills` / `log_upload`：各任务间隔（秒）
- `metrics.batch_size`：每批上报指标条数（1–100）
- `skills.scan_paths`：OpenClaw Skills 扫描路径列表
- `logs.level` / `logs.file` / `logs.batch_size`：日志级别、文件路径、每批上报条数
- `cache.dir`：离线缓存目录（不填则不用缓存）
- `retry`：重试次数与退避间隔

## 编译与运行

```bash
# 编译
go build -o agent ./cmd/agent

# 使用默认配置运行
./agent

# 指定配置文件
./agent -config=./config/config.yaml
```

跨平台编译示例：

```bash
GOOS=darwin GOARCH=amd64 go build -o agent-darwin-amd64 ./cmd/agent
GOOS=darwin GOARCH=arm64 go build -o agent-darwin-arm64 ./cmd/agent
```

## 与后端对接说明

- 注册：`POST /api/v1/agent/devices/register`，无需认证；响应中 `data.api_key` 需持久化
- 心跳 / 指标 / Skills / 日志：均在 Header 中带 `X-API-Key`
- 请求/响应格式与后端 `internal/dto/request/agent_request.go`、`internal/dto/response/agent_response.go` 一致

## 注意事项

- 首次运行会自动注册并写入 `device_id` 与 `api_key`，请勿删除
- API Key 文件权限建议为 600
- 生产环境建议使用 HTTPS
- OpenClaw Skills 元数据（版本、描述、作者等）当前为占位，可按 OpenClaw 约定扩展 `internal/collector/skills.go` 中的解析逻辑
