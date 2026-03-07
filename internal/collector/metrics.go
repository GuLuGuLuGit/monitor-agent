package collector

import (
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// MetricItem 与后端 MetricItem 一致
type MetricItem struct {
	MetricTime      time.Time `json:"metric_time"`
	CPUUsage        float64   `json:"cpu_usage"`
	MemoryUsage     float64   `json:"memory_usage"`
	MemoryUsed      int64     `json:"memory_used"`
	MemoryAvailable int64     `json:"memory_available"`
	DiskUsage       float64   `json:"disk_usage"`
	DiskUsed        int64     `json:"disk_used"`
	DiskAvailable   int64     `json:"disk_available"`
	NetworkIn       int64     `json:"network_in"`
	NetworkOut      int64     `json:"network_out"`
	LoadAverage1    float64   `json:"load_average_1"`
	LoadAverage5    float64   `json:"load_average_5"`
	LoadAverage15   float64   `json:"load_average_15"`
	ProcessCount    int       `json:"process_count"`
}

// Collect 采集一次系统指标
func Collect() (*MetricItem, error) {
	m := &MetricItem{
		MetricTime: time.Now().UTC(),
	}

	// CPU 使用率（1 秒采样）
	percent, err := cpu.Percent(time.Second, false)
	if err == nil && len(percent) > 0 {
		m.CPUUsage = percent[0]
	}

	// 内存
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		m.MemoryUsage = memInfo.UsedPercent
		m.MemoryUsed = int64(memInfo.Used)
		m.MemoryAvailable = int64(memInfo.Available)
	}

	// 磁盘（根或第一个分区）
	parts, err := disk.Partitions(false)
	if err == nil {
		for _, p := range parts {
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue
			}
			m.DiskUsage = usage.UsedPercent
			m.DiskUsed = int64(usage.Used)
			m.DiskAvailable = int64(usage.Free)
			break
		}
	}

	// 网络累计字节（所有接口）
	ioCounters, err := net.IOCounters(false)
	if err == nil && len(ioCounters) > 0 {
		m.NetworkIn = int64(ioCounters[0].BytesRecv)
		m.NetworkOut = int64(ioCounters[0].BytesSent)
	}

	// Load Average
	loadAvg, err := load.Avg()
	if err == nil {
		m.LoadAverage1 = loadAvg.Load1
		m.LoadAverage5 = loadAvg.Load5
		m.LoadAverage15 = loadAvg.Load15
	}

	// 进程数
	procs, err := process.Processes()
	if err == nil {
		m.ProcessCount = len(procs)
	}

	return m, nil
}
