package collector

import (
	"testing"
	"time"
)

func TestCollect(t *testing.T) {
	// 测试指标采集
	metric, err := Collect()
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// 验证时间戳
	if metric.MetricTime.IsZero() {
		t.Error("MetricTime should not be zero")
	}
	if time.Since(metric.MetricTime) > 5*time.Second {
		t.Error("MetricTime should be recent")
	}

	// 验证 CPU 使用率
	if metric.CPUUsage < 0 || metric.CPUUsage > 100 {
		t.Errorf("CPUUsage out of range: %f", metric.CPUUsage)
	}

	// 验证内存使用率
	if metric.MemoryUsage < 0 || metric.MemoryUsage > 100 {
		t.Errorf("MemoryUsage out of range: %f", metric.MemoryUsage)
	}
	if metric.MemoryUsed < 0 {
		t.Errorf("MemoryUsed should be non-negative: %d", metric.MemoryUsed)
	}
	if metric.MemoryAvailable < 0 {
		t.Errorf("MemoryAvailable should be non-negative: %d", metric.MemoryAvailable)
	}

	// 验证磁盘使用率
	if metric.DiskUsage < 0 || metric.DiskUsage > 100 {
		t.Errorf("DiskUsage out of range: %f", metric.DiskUsage)
	}
	if metric.DiskUsed < 0 {
		t.Errorf("DiskUsed should be non-negative: %d", metric.DiskUsed)
	}
	if metric.DiskAvailable < 0 {
		t.Errorf("DiskAvailable should be non-negative: %d", metric.DiskAvailable)
	}

	// 验证网络流量
	if metric.NetworkIn < 0 {
		t.Errorf("NetworkIn should be non-negative: %d", metric.NetworkIn)
	}
	if metric.NetworkOut < 0 {
		t.Errorf("NetworkOut should be non-negative: %d", metric.NetworkOut)
	}

	// 验证负载
	if metric.LoadAverage1 < 0 {
		t.Errorf("LoadAverage1 should be non-negative: %f", metric.LoadAverage1)
	}
	if metric.LoadAverage5 < 0 {
		t.Errorf("LoadAverage5 should be non-negative: %f", metric.LoadAverage5)
	}
	if metric.LoadAverage15 < 0 {
		t.Errorf("LoadAverage15 should be non-negative: %f", metric.LoadAverage15)
	}

	// 验证进程数
	if metric.ProcessCount <= 0 {
		t.Errorf("ProcessCount should be positive: %d", metric.ProcessCount)
	}

	t.Logf("Metric: %+v", metric)
}

func TestCollectMultipleTimes(t *testing.T) {
	// 测试多次采集
	for i := 0; i < 3; i++ {
		metric, err := Collect()
		if err != nil {
			t.Fatalf("Collect #%d failed: %v", i+1, err)
		}
		if metric == nil {
			t.Fatalf("Collect #%d returned nil", i+1)
		}
		// 等待一段时间再采集
		if i < 2 {
			time.Sleep(2 * time.Second)
		}
	}
}
