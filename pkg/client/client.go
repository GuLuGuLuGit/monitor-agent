package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"monitor-agent/pkg/logger"
)

// Client HTTP 客户端，带超时与重试
type Client struct {
	BaseURL    string
	Timeout    time.Duration
	APIKey     string
	HTTPClient *http.Client
	Retry      RetryConfig
}

type RetryConfig struct {
	MaxAttempts     int
	InitialInterval time.Duration
	MaxInterval     time.Duration
}

// APIResponse 后端统一响应
type APIResponse struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp string          `json:"timestamp"`
}

// New 创建客户端
func New(baseURL string, timeoutSec int, retry RetryConfig) *Client {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &Client{
		BaseURL: baseURL,
		Timeout: time.Duration(timeoutSec) * time.Second,
		HTTPClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
		Retry: retry,
	}
}

// SetAPIKey 设置 API Key（认证请求使用）
func (c *Client) SetAPIKey(key string) {
	c.APIKey = key
}

// Post 发送 POST 请求，带指数退避重试
func (c *Client) Post(path string, body interface{}, dataDest interface{}) error {
	var lastErr error
	interval := c.Retry.InitialInterval
	if interval <= 0 {
		interval = time.Second
	}
	maxInterval := c.Retry.MaxInterval
	if maxInterval <= 0 {
		maxInterval = 30 * time.Second
	}
	attempts := c.Retry.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}

	for attempt := 0; attempt < attempts; attempt++ {
		lastErr = c.postOnce(path, body, dataDest)
		if lastErr == nil {
			return nil
		}
		if attempt == attempts-1 {
			break
		}
		logger.Warn("request retry", "path", path, "attempt", attempt+1, "err", lastErr)
		time.Sleep(interval)
		if interval < maxInterval {
			interval *= 2
			if interval > maxInterval {
				interval = maxInterval
			}
		}
	}
	return lastErr
}

func (c *Client) postOnce(path string, body interface{}, dataDest interface{}) error {
	url := c.BaseURL + path
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(http.MethodPost, url, bodyReader)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("api error code=%d message=%s", apiResp.Code, apiResp.Message)
	}

	if dataDest != nil && len(apiResp.Data) > 0 {
		if err := json.Unmarshal(apiResp.Data, dataDest); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}
	return nil
}
