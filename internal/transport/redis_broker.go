package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"monitor-agent/pkg/logger"
)

// RedisBroker implements CommandBroker using Redis BLPOP.
// This is the legacy adapter; will be replaced by MQTTBroker in P3.
type RedisBroker struct {
	rdb *goredis.Client
}

func NewRedisBroker(rdb *goredis.Client) *RedisBroker {
	return &RedisBroker{rdb: rdb}
}

func (b *RedisBroker) Run(ctx context.Context, deviceID string, handler func(ctx context.Context, cmd *Command)) {
	queueKey := fmt.Sprintf("agent:commands:%s", deviceID)
	logger.Info("command broker started (redis)", "queue", queueKey)

	for {
		select {
		case <-ctx.Done():
			logger.Info("command broker stopped")
			return
		default:
		}

		result, err := b.rdb.BLPop(ctx, 5*time.Second, queueKey).Result()
		if err != nil {
			if err == goredis.Nil || err == context.Canceled || err == context.DeadlineExceeded {
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

		cmd, err := b.fetchCommandDetail(ctx, commandID)
		if err != nil {
			logger.Error("get command detail failed", "command_id", commandID, "err", err)
			continue
		}

		go handler(ctx, cmd)
	}
}

func (b *RedisBroker) fetchCommandDetail(ctx context.Context, commandID string) (*Command, error) {
	cmdKey := fmt.Sprintf("agent:command:%s", commandID)
	data, err := b.rdb.HGetAll(ctx, cmdKey).Result()
	if err != nil {
		return nil, fmt.Errorf("redis hgetall: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("command %s not found in redis", commandID)
	}

	var params map[string]interface{}
	paramsStr := data["command_params"]
	if paramsStr != "" && paramsStr != "map[]" {
		_ = json.Unmarshal([]byte(paramsStr), &params)
	}

	isEnc := data["is_encrypted"] == "true" || data["is_encrypted"] == "1"

	return &Command{
		ID:               commandID,
		Type:             data["command_type"],
		Params:           params,
		EncryptedPayload: data["encrypted_payload"],
		IsEncrypted:      isEnc,
	}, nil
}

func (b *RedisBroker) Close() error {
	return nil // lifecycle owned by the caller
}
