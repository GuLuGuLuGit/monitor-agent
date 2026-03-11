package transport

import "context"

// Command represents a decoded command received from the broker.
type Command struct {
	ID               string
	Type             string
	Params           map[string]interface{}
	EncryptedPayload string
	IsEncrypted      bool
}

// ResultReporter sends execution results back to the server.
type ResultReporter interface {
	ReportResult(commandID string, status int8, output, errMsg string) error
}

// Acknowledger sends command receipt acknowledgements. Implemented by MQTTBroker.
type Acknowledger interface {
	PublishAck(commandID string) error
}

// ProgressReport represents a point-in-time progress update for a running command.
type ProgressReport struct {
	CommandID      int64   `json:"command_id"`
	Status         string  `json:"status"`
	Progress       int     `json:"progress"`
	CurrentStep    string  `json:"current_step"`
	TotalSteps     int     `json:"total_steps"`
	CompletedSteps int     `json:"completed_steps"`
	SnapshotURL    *string `json:"snapshot_url,omitempty"`
	Timestamp      int64   `json:"timestamp"`
}

// ProgressReporter publishes task progress updates back to the server.
type ProgressReporter interface {
	PublishProgress(report *ProgressReport) error
}

// CommandBroker abstracts how the agent receives commands from the server.
// Implementations: RedisBroker (legacy), MQTTBroker (new).
type CommandBroker interface {
	// Run blocks and listens for incoming commands until ctx is cancelled.
	// For each received command, handler is called in a new goroutine.
	Run(ctx context.Context, deviceID string, handler func(ctx context.Context, cmd *Command))

	// Close releases underlying resources.
	Close() error
}
