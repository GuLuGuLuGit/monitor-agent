package uploader

import (
	"time"

	"monitor-agent/internal/collector"
	"monitor-agent/internal/device"
	"monitor-agent/pkg/client"
	"monitor-agent/pkg/logger"
)

const (
	apiPrefix     = "/api/v1/agent"
	registerPath  = apiPrefix + "/devices/register"
	heartbeatPath = apiPrefix + "/devices/heartbeat"
	metricsPath   = apiPrefix + "/metrics"
	skillsPath    = apiPrefix + "/skills"
	logsPath      = apiPrefix + "/logs"
)

// Uploader 上报器：注册、心跳、指标、Skills、日志
type Uploader struct {
	client *client.Client
}

func New(c *client.Client) *Uploader {
	return &Uploader{client: c}
}

// Register 设备注册，返回 API Key
func (u *Uploader) Register(info *device.Info) (*device.RegisterResponse, error) {
	req := info.ToRegisterRequest()
	var resp device.RegisterResponse
	// 注册不带 API Key
	origKey := u.client.APIKey
	u.client.SetAPIKey("")
	defer u.client.SetAPIKey(origKey)

	if err := u.client.Post(registerPath, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Heartbeat 心跳；extraData 为可选的 JSON 扩展数据
func (u *Uploader) Heartbeat(info *device.Info, agentVersion string, status int8, publicKeyPEM string, extraData *string) (*device.HeartbeatResponse, error) {
	req := device.HeartbeatRequest{
		AgentVersion: agentVersion,
		Status:       status,
		PublicKeyPEM: publicKeyPEM,
		ExtraData:    extraData,
	}
	if info != nil {
		req.CPUModel = info.CPUModel
		req.CPUCores = info.CPUCores
		req.MemoryTotal = info.MemoryTotal
		req.DiskTotal = info.DiskTotal
	}
	var resp device.HeartbeatResponse
	if err := u.client.Post(heartbeatPath, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UploadMetrics 批量上报指标
func (u *Uploader) UploadMetrics(metrics []collector.MetricItem) error {
	if len(metrics) == 0 {
		return nil
	}
	req := struct {
		Metrics []collector.MetricItem `json:"metrics"`
	}{Metrics: metrics}
	var resp struct {
		ReceivedCount  int `json:"received_count"`
		SavedCount     int `json:"saved_count"`
		DuplicateCount int `json:"duplicate_count"`
	}
	if err := u.client.Post(metricsPath, req, &resp); err != nil {
		return err
	}
	logger.Debug("metrics uploaded", "count", resp.SavedCount)
	return nil
}

// UploadSkills 上报 Skills
func (u *Uploader) UploadSkills(skills []collector.SkillItem) error {
	if len(skills) == 0 {
		return nil
	}
	req := struct {
		ScanTime time.Time             `json:"scan_time"`
		Skills   []collector.SkillItem `json:"skills"`
	}{
		ScanTime: time.Now().UTC(),
		Skills:   skills,
	}
	var resp struct {
		ReceivedCount int `json:"received_count"`
		SavedCount    int `json:"saved_count"`
		UpdatedCount  int `json:"updated_count"`
	}
	if err := u.client.Post(skillsPath, req, &resp); err != nil {
		return err
	}
	logger.Debug("skills uploaded", "count", resp.SavedCount)
	return nil
}

// UploadLogs 批量上报日志
func (u *Uploader) UploadLogs(batchID string, logs []logger.LogEntry) error {
	if len(logs) == 0 {
		return nil
	}
	type logItem struct {
		LogLevel   string    `json:"log_level"`
		LogSource  string    `json:"log_source"`
		LogMessage string    `json:"log_message"`
		LogTime    time.Time `json:"log_time"`
		Sequence   int       `json:"sequence"`
	}
	items := make([]logItem, len(logs))
	for i := range logs {
		logTime, _ := time.Parse(time.RFC3339, logs[i].Time)
		items[i] = logItem{
			LogLevel:   logs[i].Level,
			LogSource:  logs[i].Source,
			LogMessage: logs[i].Message,
			LogTime:    logTime,
			Sequence:   int(logs[i].Sequence),
		}
	}
	req := struct {
		BatchID string    `json:"batch_id"`
		Logs    []logItem `json:"logs"`
	}{
		BatchID: batchID,
		Logs:    items,
	}
	var resp struct {
		ReceivedCount int `json:"received_count"`
		SavedCount    int `json:"saved_count"`
	}
	if err := u.client.Post(logsPath, req, &resp); err != nil {
		return err
	}
	logger.Debug("logs uploaded", "count", resp.SavedCount)
	return nil
}
