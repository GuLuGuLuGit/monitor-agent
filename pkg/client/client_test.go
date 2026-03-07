package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	retry := RetryConfig{
		MaxAttempts:     3,
		InitialInterval: time.Second,
		MaxInterval:     10 * time.Second,
	}
	client := New("http://localhost:8080", 30, retry)

	if client.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL mismatch")
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("Timeout mismatch")
	}
	if client.Retry.MaxAttempts != 3 {
		t.Errorf("MaxAttempts mismatch")
	}
}

func TestSetAPIKey(t *testing.T) {
	client := New("http://localhost:8080", 30, RetryConfig{})
	testKey := "test-api-key"

	client.SetAPIKey(testKey)

	if client.APIKey != testKey {
		t.Errorf("API Key not set correctly")
	}
}

func TestPostSuccess(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// 验证 Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected application/json content type")
		}

		// 验证 API Key
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("API Key header mismatch")
		}

		// 返回成功响应
		resp := APIResponse{
			Code:      0,
			Message:   "success",
			Data:      json.RawMessage(`{"result":"ok"}`),
			Timestamp: time.Now().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// 创建客户端
	client := New(server.URL, 5, RetryConfig{MaxAttempts: 1})
	client.SetAPIKey("test-key")

	// 发送请求
	reqBody := map[string]string{"test": "data"}
	var respData map[string]string
	err := client.Post("/test", reqBody, &respData)

	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}

	if respData["result"] != "ok" {
		t.Errorf("Response data mismatch")
	}
}

func TestPostError(t *testing.T) {
	// 创建返回错误的测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := APIResponse{
			Code:      400,
			Message:   "bad request",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(server.URL, 5, RetryConfig{MaxAttempts: 1})

	err := client.Post("/test", nil, nil)

	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestPostRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// 前两次返回错误
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// 第三次返回成功
		resp := APIResponse{
			Code:      0,
			Message:   "success",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(server.URL, 5, RetryConfig{
		MaxAttempts:     3,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     time.Second,
	})

	err := client.Post("/test", nil, nil)

	if err != nil {
		t.Fatalf("Post should succeed after retries: %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}
