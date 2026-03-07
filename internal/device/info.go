package device

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// Info 设备信息（用于注册）
type Info struct {
	DeviceID     string
	Hostname     string
	MacAddress   string
	OSVersion    string
	CPUModel     string
	CPUCores     int
	MemoryTotal  int64
	DiskTotal    int64
	AgentVersion string
}

const AgentVersion = "1.0.0"

// Collect 采集设备信息；deviceID 若为空则生成新 UUID
func Collect(deviceID string) (*Info, error) {
	info := &Info{
		AgentVersion: AgentVersion,
	}

	if deviceID != "" {
		info.DeviceID = deviceID
	} else {
		info.DeviceID = uuid.New().String()
	}

	hostInfo, err := host.Info()
	if err != nil {
		return nil, fmt.Errorf("host info: %w", err)
	}
	info.Hostname = hostInfo.Hostname
	info.OSVersion = hostInfo.Platform + " " + hostInfo.PlatformVersion
	if info.OSVersion == " " {
		info.OSVersion = runtime.GOOS
	}

	// MAC：取第一个非空物理网卡
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) > 0 && (iface.Flags == nil || len(iface.Flags) > 0) {
				info.MacAddress = iface.HardwareAddr
				break
			}
		}
	}
	info.MacAddress = normalizeMAC(info.MacAddress)
	if info.MacAddress == "" {
		info.MacAddress = "00:00:00:00:00:00"
	}

	// CPU
	cpuInfos, err := cpu.Info()
	if err == nil && len(cpuInfos) > 0 {
		info.CPUModel = cpuInfos[0].ModelName
		// 使用逻辑核心数
		info.CPUCores = len(cpuInfos)
	}
	if info.CPUCores == 0 {
		info.CPUCores = runtime.NumCPU()
	}

	// 内存
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		info.MemoryTotal = int64(memInfo.Total)
	}

	// 磁盘（根分区或第一块）
	parts, err := disk.Partitions(false)
	if err == nil {
		for _, p := range parts {
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue
			}
			info.DiskTotal = int64(usage.Total)
			break
		}
	}

	return info, nil
}

// normalizeMAC 转为后端校验所需的 xx:xx:xx:xx:xx:xx 格式
func normalizeMAC(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", ":")
	s = strings.ToLower(s)
	if strings.Count(s, ":") == 5 && len(s) == 17 {
		return s
	}
	// 只保留十六进制字符
	var hex []byte
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			hex = append(hex, byte(r))
		}
	}
	if len(hex) < 12 {
		return s
	}
	hex = hex[:12]
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		string(hex[0:2]), string(hex[2:4]), string(hex[4:6]),
		string(hex[6:8]), string(hex[8:10]), string(hex[10:12]))
}
