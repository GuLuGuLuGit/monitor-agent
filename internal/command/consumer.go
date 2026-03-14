package command

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"monitor-agent/internal/openclawstate"
	"monitor-agent/internal/transport"
	"monitor-agent/pkg/crypto"
	"monitor-agent/pkg/logger"
)

// Consumer receives and executes commands from any CommandBroker implementation.
type Consumer struct {
	broker     transport.CommandBroker
	deviceID   string
	reporter   transport.ResultReporter
	acker      transport.Acknowledger
	progress   transport.ProgressReporter
	privateKey *rsa.PrivateKey
}

// NewConsumer creates a new Consumer backed by the given broker and result reporter.
// privateKey is used for decrypting E2E encrypted commands (may be nil to skip decryption).
// acker is optional; if provided, an ACK is sent on command receipt.
// progress is optional; if provided, progress updates are published during execution.
func NewConsumer(broker transport.CommandBroker, deviceID string, reporter transport.ResultReporter, privateKey *rsa.PrivateKey, acker transport.Acknowledger, progress transport.ProgressReporter) *Consumer {
	return &Consumer{
		broker:     broker,
		deviceID:   deviceID,
		reporter:   reporter,
		acker:      acker,
		progress:   progress,
		privateKey: privateKey,
	}
}

// Run blocks and processes commands until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) {
	c.broker.Run(ctx, c.deviceID, c.handleCommand)
}

func (c *Consumer) handleCommand(ctx context.Context, cmd *transport.Command) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("command execution panic", "command_id", cmd.ID, "recover", r)
			c.reportResult(cmd.ID, 3, "", fmt.Sprintf("panic: %v", r))
		}
	}()

	if c.acker != nil {
		if err := c.acker.PublishAck(cmd.ID); err != nil {
			logger.Warn("send ACK failed", "command_id", cmd.ID, "err", err)
		}
	}

	cmdType := cmd.Type
	cmdParams := cmd.Params

	if cmd.IsEncrypted && cmd.EncryptedPayload != "" {
		if c.privateKey == nil {
			logger.Error("encrypted command received but no private key", "command_id", cmd.ID)
			c.reportResult(cmd.ID, 3, "", "no private key for decryption")
			return
		}

		var envelope crypto.Envelope
		if err := json.Unmarshal([]byte(cmd.EncryptedPayload), &envelope); err != nil {
			logger.Error("parse encrypted envelope", "command_id", cmd.ID, "err", err)
			c.reportResult(cmd.ID, 3, "", fmt.Sprintf("parse envelope: %v", err))
			return
		}

		plaintext, err := crypto.Open(c.privateKey, &envelope)
		if err != nil {
			logger.Error("decrypt command", "command_id", cmd.ID, "err", err)
			c.reportResult(cmd.ID, 3, "", fmt.Sprintf("decrypt: %v", err))
			return
		}

		var decrypted struct {
			CommandType   string                 `json:"command_type"`
			CommandParams map[string]interface{} `json:"command_params"`
			Params        map[string]interface{} `json:"params"`
		}
		if err := json.Unmarshal(plaintext, &decrypted); err != nil {
			logger.Error("unmarshal decrypted command", "command_id", cmd.ID, "err", err)
			c.reportResult(cmd.ID, 3, "", fmt.Sprintf("unmarshal decrypted: %v", err))
			return
		}

		cmdType = decrypted.CommandType
		cmdParams = decrypted.CommandParams
		if len(cmdParams) == 0 && len(decrypted.Params) > 0 {
			cmdParams = decrypted.Params
		}
		logger.Info("command decrypted", "command_id", cmd.ID, "type", cmdType)
	}

	c.reportResult(cmd.ID, 1, "", "")
	c.sendProgress(cmd.ID, "running", 0, "开始执行", 1, 0)
	if cmdType == TypeMessage {
		openclawstate.MarkMessageActivity()
	}

	result := Execute(cmdType, cmdParams)

	finalStatus := "completed"
	if result.Status == 3 {
		finalStatus = "failed"
	}
	c.sendProgress(cmd.ID, finalStatus, 100, "执行完成", 1, 1)
	c.reportResult(cmd.ID, result.Status, result.Output, result.ErrorMessage)
}

func (c *Consumer) sendProgress(commandID string, status string, progress int, step string, totalSteps int, completedSteps int) {
	if c.progress == nil {
		return
	}
	var id int64
	fmt.Sscanf(commandID, "%d", &id)
	if id == 0 {
		return
	}
	report := &transport.ProgressReport{
		CommandID:      id,
		Status:         status,
		Progress:       progress,
		CurrentStep:    step,
		TotalSteps:     totalSteps,
		CompletedSteps: completedSteps,
		Timestamp:      time.Now().Unix(),
	}
	if err := c.progress.PublishProgress(report); err != nil {
		logger.Warn("publish progress failed", "command_id", commandID, "err", err)
	}
}

func (c *Consumer) reportResult(commandID string, status int8, output, errMsg string) {
	if status != 1 {
		logger.Info("reporting command result", "command_id", commandID, "status", status)
	}

	if err := c.reporter.ReportResult(commandID, status, output, errMsg); err != nil {
		logger.Warn("report command result failed", "command_id", commandID, "err", err)
	}
}
