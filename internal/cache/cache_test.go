package cache

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()

	c, err := New(tmpDir, 10)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if c.dir != tmpDir {
		t.Errorf("Dir mismatch")
	}
	if c.maxBytes != 10*1024*1024 {
		t.Errorf("MaxBytes mismatch")
	}
}

func TestPushAndPop(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 10)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 测试推送指标
	testMetric := map[string]interface{}{
		"cpu_usage":    50.5,
		"memory_usage": 60.0,
	}

	err = c.Push(KindMetrics, testMetric)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// 等待一小段时间确保文件写入
	time.Sleep(100 * time.Millisecond)

	// 测试弹出指标
	metrics, err := c.PopMetrics(10)
	if err != nil {
		t.Fatalf("PopMetrics failed: %v", err)
	}

	if len(metrics) != 1 {
		t.Errorf("Expected 1 metric, got %d", len(metrics))
	}

	// 验证数据
	var retrieved map[string]interface{}
	err = json.Unmarshal(metrics[0], &retrieved)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if retrieved["cpu_usage"] != 50.5 {
		t.Errorf("CPU usage mismatch")
	}
}

func TestPushMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 10)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 推送多个条目
	for i := 0; i < 5; i++ {
		testData := map[string]int{"value": i}
		err = c.Push(KindMetrics, testData)
		if err != nil {
			t.Fatalf("Push #%d failed: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond) // 确保文件名不重复
	}

	time.Sleep(100 * time.Millisecond)

	// 弹出所有条目
	metrics, err := c.PopMetrics(10)
	if err != nil {
		t.Fatalf("PopMetrics failed: %v", err)
	}

	if len(metrics) < 3 {
		t.Errorf("Expected at least 3 metrics, got %d", len(metrics))
	}

	// 再次弹出应该为空
	metrics2, err := c.PopMetrics(10)
	if err != nil {
		t.Fatalf("PopMetrics failed: %v", err)
	}

	if len(metrics2) != 0 {
		t.Errorf("Expected 0 metrics after pop, got %d", len(metrics2))
	}
}

func TestPushLogs(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 10)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 推送日志
	testLog := map[string]string{
		"level":   "INFO",
		"message": "test log",
	}

	err = c.Push(KindLogs, testLog)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 弹出日志
	logs, err := c.PopLogs(10)
	if err != nil {
		t.Fatalf("PopLogs failed: %v", err)
	}

	if len(logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs))
	}
}

func TestMaxSize(t *testing.T) {
	tmpDir := t.TempDir()
	// 设置很小的最大大小
	c, err := New(tmpDir, 1) // 1MB
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// 推送大量数据
	largeData := make([]byte, 100*1024) // 100KB
	for i := 0; i < 20; i++ {
		err = c.Push(KindMetrics, largeData)
		if err != nil {
			// 预期在某个时候会因为空间不足而失败或删除旧数据
			t.Logf("Push #%d: %v", i, err)
		}
	}

	// 验证缓存大小没有超过限制
	if c.written > c.maxBytes*2 {
		t.Errorf("Cache size exceeded limit significantly: %d > %d", c.written, c.maxBytes)
	}
}
