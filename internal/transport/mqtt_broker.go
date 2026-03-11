package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"monitor-agent/pkg/logger"
)

const (
	mqttTopicPrefix = "openclaw"
	mqttQoS         byte = 1
)

// MQTTBrokerConfig holds the parameters needed to create an agent-side MQTTBroker.
type MQTTBrokerConfig struct {
	BrokerURL     string
	DeviceID      string
	KeepAlive     int
	AutoReconnect bool
	Username      string
	Password      string
	UseTLS        bool
}

// MQTTBroker implements CommandBroker and ResultReporter over MQTT.
// It subscribes to the device's command topic, publishes results, and
// manages presence (online/offline) via retained messages and LWT.
type MQTTBroker struct {
	client   mqtt.Client
	deviceID string
	cmdCh    chan *Command
}

var (
	_ CommandBroker    = (*MQTTBroker)(nil)
	_ ResultReporter   = (*MQTTBroker)(nil)
	_ ProgressReporter = (*MQTTBroker)(nil)
)

func NewMQTTBroker(cfg MQTTBrokerConfig) (*MQTTBroker, error) {
	b := &MQTTBroker{
		deviceID: cfg.DeviceID,
		cmdCh:    make(chan *Command, 32),
	}

	statusTopic := fmt.Sprintf("%s/%s/status", mqttTopicPrefix, cfg.DeviceID)

	keepAlive := cfg.KeepAlive
	if keepAlive <= 0 {
		keepAlive = 60
	}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.BrokerURL).
		SetClientID(fmt.Sprintf("agent-%s", cfg.DeviceID)).
		SetKeepAlive(time.Duration(keepAlive) * time.Second).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(30 * time.Second).
		SetCleanSession(false).
		SetWill(statusTopic, "offline", mqttQoS, true)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}
	if cfg.UseTLS {
		opts.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	}

	opts.
		SetOnConnectHandler(func(c mqtt.Client) {
			logger.Info("MQTT connected, publishing online status")
			c.Publish(statusTopic, mqttQoS, true, "online")

			cmdTopic := fmt.Sprintf("%s/%s/cmd", mqttTopicPrefix, cfg.DeviceID)
			if token := c.Subscribe(cmdTopic, mqttQoS, b.handleCommand); token.Wait() && token.Error() != nil {
				logger.Error("subscribe command topic failed", "err", token.Error())
			} else {
				logger.Info("subscribed to command topic", "topic", cmdTopic)
			}
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			logger.Warn("MQTT connection lost", "err", err)
		})

	b.client = mqtt.NewClient(opts)

	token := b.client.Connect()
	token.Wait()
	if token.Error() != nil {
		return nil, fmt.Errorf("mqtt connect: %w", token.Error())
	}

	return b, nil
}

func (b *MQTTBroker) handleCommand(_ mqtt.Client, msg mqtt.Message) {
	var payload struct {
		CommandID        string `json:"command_id"`
		Type             string `json:"command_type"`
		Params           string `json:"command_params"`
		EncryptedPayload string `json:"encrypted_payload"`
		IsEncrypted      bool   `json:"is_encrypted"`
	}
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		logger.Warn("unmarshal MQTT command", "err", err)
		return
	}

	var params map[string]interface{}
	if payload.Params != "" {
		_ = json.Unmarshal([]byte(payload.Params), &params)
	}

	logger.Info("received command via MQTT",
		"command_id", payload.CommandID,
		"type", payload.Type,
		"encrypted", payload.IsEncrypted)

	b.cmdCh <- &Command{
		ID:               payload.CommandID,
		Type:             payload.Type,
		Params:           params,
		EncryptedPayload: payload.EncryptedPayload,
		IsEncrypted:      payload.IsEncrypted,
	}
}

// Run blocks and dispatches incoming commands to the handler until ctx is cancelled.
func (b *MQTTBroker) Run(ctx context.Context, deviceID string, handler func(ctx context.Context, cmd *Command)) {
	logger.Info("command broker started (mqtt)", "device_id", deviceID)
	for {
		select {
		case <-ctx.Done():
			logger.Info("command broker stopped (mqtt)")
			return
		case cmd := <-b.cmdCh:
			go handler(ctx, cmd)
		}
	}
}

// ReportResult publishes a command result to the server via MQTT.
func (b *MQTTBroker) ReportResult(commandID string, status int8, output, errMsg string) error {
	topic := fmt.Sprintf("%s/%s/result", mqttTopicPrefix, b.deviceID)
	payload := struct {
		CommandID    string `json:"command_id"`
		Status       int8   `json:"status"`
		Output       string `json:"output"`
		ErrorMessage string `json:"error_message"`
	}{
		CommandID:    commandID,
		Status:       status,
		Output:       output,
		ErrorMessage: errMsg,
	}

	data, _ := json.Marshal(payload)
	token := b.client.Publish(topic, mqttQoS, false, data)
	token.Wait()
	if token.Error() != nil {
		logger.Warn("publish result via MQTT failed", "command_id", commandID, "err", token.Error())
		return token.Error()
	}
	return nil
}

// PublishAck sends a command acknowledgement to the server.
func (b *MQTTBroker) PublishAck(commandID string) error {
	topic := fmt.Sprintf("%s/%s/ack", mqttTopicPrefix, b.deviceID)
	payload := struct {
		CommandID string `json:"command_id"`
	}{CommandID: commandID}

	data, _ := json.Marshal(payload)
	token := b.client.Publish(topic, mqttQoS, false, data)
	token.Wait()
	return token.Error()
}

// PublishProgress sends a task progress update to the server via MQTT.
func (b *MQTTBroker) PublishProgress(report *ProgressReport) error {
	topic := fmt.Sprintf("%s/%s/progress", mqttTopicPrefix, b.deviceID)
	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}
	token := b.client.Publish(topic, mqttQoS, false, data)
	token.Wait()
	if token.Error() != nil {
		logger.Warn("publish progress via MQTT failed", "command_id", report.CommandID, "err", token.Error())
		return token.Error()
	}
	return nil
}

func (b *MQTTBroker) Close() error {
	statusTopic := fmt.Sprintf("%s/%s/status", mqttTopicPrefix, b.deviceID)
	token := b.client.Publish(statusTopic, mqttQoS, true, "offline")
	token.Wait()
	b.client.Disconnect(1000)
	return nil
}
