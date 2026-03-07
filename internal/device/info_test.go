package device

import (
	"testing"
)

func TestCollect(t *testing.T) {
	// 测试设备信息采集
	info, err := Collect("")
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// 验证必填字段
	if info.DeviceID == "" {
		t.Error("DeviceID should not be empty")
	}
	if info.Hostname == "" {
		t.Error("Hostname should not be empty")
	}
	if info.MacAddress == "" {
		t.Error("MacAddress should not be empty")
	}
	if info.CPUCores <= 0 {
		t.Error("CPUCores should be positive")
	}
	if info.MemoryTotal <= 0 {
		t.Error("MemoryTotal should be positive")
	}
	if info.AgentVersion != AgentVersion {
		t.Errorf("AgentVersion mismatch: got %s, want %s", info.AgentVersion, AgentVersion)
	}

	t.Logf("Device Info: %+v", info)
}

func TestCollectWithExistingID(t *testing.T) {
	// 测试使用已有 DeviceID
	existingID := "test-device-id-123"
	info, err := Collect(existingID)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if info.DeviceID != existingID {
		t.Errorf("DeviceID mismatch: got %s, want %s", info.DeviceID, existingID)
	}
}

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard format",
			input:    "00:11:22:33:44:55",
			expected: "00:11:22:33:44:55",
		},
		{
			name:     "with dashes",
			input:    "00-11-22-33-44-55",
			expected: "00:11:22:33:44:55",
		},
		{
			name:     "uppercase",
			input:    "AA:BB:CC:DD:EE:FF",
			expected: "aa:bb:cc:dd:ee:ff",
		},
		{
			name:     "no separators",
			input:    "aabbccddeeff",
			expected: "aa:bb:cc:dd:ee:ff",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeMAC(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeMAC(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToRegisterRequest(t *testing.T) {
	info := &Info{
		DeviceID:     "test-id",
		Hostname:     "test-host",
		MacAddress:   "00:11:22:33:44:55",
		OSVersion:    "macOS 14.0",
		CPUModel:     "Apple M1",
		CPUCores:     8,
		MemoryTotal:  16 * 1024 * 1024 * 1024,
		DiskTotal:    512 * 1024 * 1024 * 1024,
		AgentVersion: "1.0.0",
	}

	req := info.ToRegisterRequest()

	if req.DeviceID != info.DeviceID {
		t.Errorf("DeviceID mismatch")
	}
	if req.Hostname != info.Hostname {
		t.Errorf("Hostname mismatch")
	}
	if req.MacAddress != info.MacAddress {
		t.Errorf("MacAddress mismatch")
	}
	if req.CPUCores != info.CPUCores {
		t.Errorf("CPUCores mismatch")
	}
}
