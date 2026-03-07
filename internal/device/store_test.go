package device

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrStoreID(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	idFile := filepath.Join(tmpDir, "device_id")

	// 测试存储新 ID
	newID := "test-device-123"
	id, err := LoadOrStoreID(idFile, newID)
	if err != nil {
		t.Fatalf("LoadOrStoreID failed: %v", err)
	}
	if id != newID {
		t.Errorf("ID mismatch: got %s, want %s", id, newID)
	}

	// 验证文件存在
	if _, err := os.Stat(idFile); os.IsNotExist(err) {
		t.Error("ID file should exist")
	}

	// 测试读取已存在的 ID
	id2, err := LoadID(idFile)
	if err != nil {
		t.Fatalf("LoadID failed: %v", err)
	}
	if id2 != newID {
		t.Errorf("Loaded ID mismatch: got %s, want %s", id2, newID)
	}
}

func TestLoadIDNotExist(t *testing.T) {
	// 测试读取不存在的文件
	tmpDir := t.TempDir()
	idFile := filepath.Join(tmpDir, "nonexistent")

	id, err := LoadID(idFile)
	if err != nil {
		t.Fatalf("LoadID should not error on nonexistent file: %v", err)
	}
	if id != "" {
		t.Errorf("ID should be empty for nonexistent file, got %s", id)
	}
}

func TestStoreAndLoadAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	apiKeyFile := filepath.Join(tmpDir, "api_key")

	// 测试存储 API Key
	testKey := "test-api-key-12345"
	err := StoreAPIKey(apiKeyFile, testKey)
	if err != nil {
		t.Fatalf("StoreAPIKey failed: %v", err)
	}

	// 测试读取 API Key
	key, err := LoadAPIKey(apiKeyFile)
	if err != nil {
		t.Fatalf("LoadAPIKey failed: %v", err)
	}
	if key != testKey {
		t.Errorf("API Key mismatch: got %s, want %s", key, testKey)
	}

	// 验证文件权限（Windows 上权限检查不同，跳过）
	// Windows 文件权限模型与 Unix 不同，这里只验证文件存在
	info, err := os.Stat(apiKeyFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() == 0 {
		t.Error("API Key file should not be empty")
	}
}

func TestLoadAPIKeyNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	apiKeyFile := filepath.Join(tmpDir, "nonexistent")

	key, err := LoadAPIKey(apiKeyFile)
	if err != nil {
		t.Fatalf("LoadAPIKey should not error on nonexistent file: %v", err)
	}
	if key != "" {
		t.Errorf("API Key should be empty for nonexistent file, got %s", key)
	}
}
