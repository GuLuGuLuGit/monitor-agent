package device

// RegisterRequest 与后端 DeviceRegisterRequest 字段一致
type RegisterRequest struct {
	DeviceID     string `json:"device_id"`
	Hostname     string `json:"hostname"`
	MacAddress   string `json:"mac_address"`
	OSVersion    string `json:"os_version"`
	CPUModel     string `json:"cpu_model"`
	CPUCores     int    `json:"cpu_cores"`
	MemoryTotal  int64  `json:"memory_total"`
	DiskTotal    int64  `json:"disk_total"`
	AgentVersion string `json:"agent_version"`
}

// RegisterResponse 与后端 DeviceRegisterResponse 一致
type RegisterResponse struct {
	DeviceID string `json:"device_id"`
	APIKey   string `json:"api_key"`
	Status   int8   `json:"status"`
}

// HeartbeatRequest 与后端 DeviceHeartbeatRequest 一致
type HeartbeatRequest struct {
	AgentVersion string  `json:"agent_version"`
	Status       int8    `json:"status"`
	CPUModel     string  `json:"cpu_model,omitempty"`
	CPUCores     int     `json:"cpu_cores,omitempty"`
	MemoryTotal  int64   `json:"memory_total,omitempty"`
	DiskTotal    int64   `json:"disk_total,omitempty"`
	ExtraData    *string `json:"extra_data,omitempty"`
}

// HeartbeatResponse 与后端 DeviceHeartbeatResponse 一致
type HeartbeatResponse struct {
	ServerTime            string `json:"server_time"`
	NextHeartbeatInterval int    `json:"next_heartbeat_interval"`
}

// ToRegisterRequest 从 Info 转为注册请求
func (i *Info) ToRegisterRequest() RegisterRequest {
	return RegisterRequest{
		DeviceID:     i.DeviceID,
		Hostname:     i.Hostname,
		MacAddress:   i.MacAddress,
		OSVersion:    i.OSVersion,
		CPUModel:     i.CPUModel,
		CPUCores:     i.CPUCores,
		MemoryTotal:  i.MemoryTotal,
		DiskTotal:    i.DiskTotal,
		AgentVersion: i.AgentVersion,
	}
}
