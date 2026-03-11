package transport

import (
	"fmt"

	"monitor-agent/pkg/client"
	"monitor-agent/pkg/logger"
)

const commandResultPath = "/api/v1/agent/commands/result"

// HTTPResultReporter reports command results via HTTP POST (legacy path).
type HTTPResultReporter struct {
	http *client.Client
}

func NewHTTPResultReporter(c *client.Client) *HTTPResultReporter {
	return &HTTPResultReporter{http: c}
}

func (r *HTTPResultReporter) ReportResult(commandID string, status int8, output, errMsg string) error {
	var id int64
	fmt.Sscanf(commandID, "%d", &id)
	if id == 0 {
		return fmt.Errorf("invalid command id: %s", commandID)
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

	if err := r.http.Post(commandResultPath, req, nil); err != nil {
		logger.Warn("report result via HTTP failed", "command_id", commandID, "err", err)
		return err
	}
	return nil
}
