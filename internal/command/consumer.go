package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"monitor-agent/pkg/client"
	"monitor-agent/pkg/logger"
)

const (
	commandResultPath = "/api/v1/agent/commands/result"
)

// Consumer 从 Redis 队列消费并执行命令
type Consumer struct {
	rdb      *redis.Client
	deviceID string
	http     *client.Client
}

// NewConsumer 创建命令消费者
func NewConsumer(rdb *redis.Client, deviceID string, httpClient *client.Client) *Consumer {
	return &Consumer{
		rdb:      rdb,
		deviceID: deviceID,
		http:     httpClient,
	}
}

// Run 阻塞运行命令消费循环，直到 ctx 取消
func (c *Consumer) Run(ctx context.Context) {
	queueKey := fmt.Sprintf("agent:commands:%s", c.deviceID)
	logger.Info("command consumer started", "queue", queueKey)

	for {
		select {
		case <-ctx.Done():
			logger.Info("command consumer stopped")
			return
		default:
		}

		// BLPOP 最多等 5 秒，避免长时间阻塞导致无法响应 ctx 取消
		result, err := c.rdb.BLPop(ctx, 5*time.Second, queueKey).Result()
		if err != nil {
			if err == redis.Nil || err == context.Canceled || err == context.DeadlineExceeded {
				continue
			}
			logger.Warn("blpop error", "err", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if len(result) < 2 {
			continue
		}
		commandID := result[1]
		logger.Info("received command", "command_id", commandID)

		go c.processCommand(ctx, commandID)
	}
}

func (c *Consumer) processCommand(ctx context.Context, commandID string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("command execution panic", "command_id", commandID, "recover", r)
			c.reportResult(commandID, 3, "", fmt.Sprintf("panic: %v", r))
		}
	}()

	// 从 Redis 获取命令详情
	cmdKey := fmt.Sprintf("agent:command:%s", commandID)
	data, err := c.rdb.HGetAll(ctx, cmdKey).Result()
	if err != nil || len(data) == 0 {
		logger.Error("get command detail failed", "command_id", commandID, "err", err)
		c.reportResult(commandID, 3, "", "failed to get command detail from redis")
		return
	}

	commandType := data["command_type"]
	paramsStr := data["command_params"]

	var params map[string]interface{}
	if paramsStr != "" && paramsStr != "map[]" {
		_ = json.Unmarshal([]byte(paramsStr), &params)
	}

	// 更新状态为执行中
	c.reportResult(commandID, 1, "", "")

	// 执行命令
	result := Execute(commandType, params)

	// 上报结果
	c.reportResult(commandID, result.Status, result.Output, result.ErrorMessage)
}

func (c *Consumer) reportResult(commandIDStr string, status int8, output, errMsg string) {
	// 跳过"执行中"状态以外的日志，"执行中"是正常流转
	if status != 1 {
		logger.Info("reporting command result", "command_id", commandIDStr, "status", status)
	}

	var id int64
	fmt.Sscanf(commandIDStr, "%d", &id)
	if id == 0 {
		logger.Error("invalid command id", "raw", commandIDStr)
		return
	}

	req := struct {
		ID           int64  `json:"id"`
		Status       int8   `json:"status"`
		Result       string `json:"result"`
		ErrorMessage string `json:"error_message"`
	}{
		ID:           id,
		Status:       status,
		Result:       output,
		ErrorMessage: errMsg,
	}

	if err := c.http.Post(commandResultPath, req, nil); err != nil {
		logger.Warn("report command result failed", "command_id", commandIDStr, "err", err)
	}
}
