package device

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadOrStoreID 从文件读取 DeviceID；若文件不存在则返回空，由调用方生成并调用 StoreID 保存
func LoadOrStoreID(idFile, newID string) (string, error) {
	if newID != "" {
		dir := filepath.Dir(idFile)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(idFile, []byte(strings.TrimSpace(newID)), 0600); err != nil {
			return "", err
		}
		return newID, nil
	}
	data, err := os.ReadFile(idFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// LoadID 仅读取
func LoadID(idFile string) (string, error) {
	data, err := os.ReadFile(idFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// StoreID 保存 DeviceID
func StoreID(idFile, id string) error {
	dir := filepath.Dir(idFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(idFile, []byte(strings.TrimSpace(id)), 0600)
}

// LoadAPIKey 读取 API Key
func LoadAPIKey(apiKeyFile string) (string, error) {
	data, err := os.ReadFile(apiKeyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// StoreAPIKey 保存 API Key
func StoreAPIKey(apiKeyFile, key string) error {
	dir := filepath.Dir(apiKeyFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(apiKeyFile, []byte(strings.TrimSpace(key)), 0600)
}
