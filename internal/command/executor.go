package command

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"monitor-agent/pkg/logger"
)

const (
	TypeStart   = "openclaw_start"
	TypeStop    = "openclaw_stop"
	TypeRestart = "openclaw_restart"
	TypeStatus  = "openclaw_status"
	TypeConfig  = "openclaw_config"

	serviceName = "ai.openclaw.gateway"
)

// Result 命令执行结果
type Result struct {
	Status       int8   `json:"status"`
	Output       string `json:"result"`
	ErrorMessage string `json:"error_message"`
}

// Execute 根据命令类型执行对应操作
func Execute(commandType string, params map[string]interface{}) *Result {
	logger.Info("executing command", "type", commandType)
	start := time.Now()

	var result *Result
	switch commandType {
	case TypeStart:
		result = execLaunchctl("start")
	case TypeStop:
		result = execLaunchctl("stop")
	case TypeRestart:
		result = execRestart()
	case TypeStatus:
		result = execStatus()
	case TypeConfig:
		result = execConfig(params)
	default:
		result = &Result{
			Status:       3,
			ErrorMessage: fmt.Sprintf("unknown command type: %s", commandType),
		}
	}

	logger.Info("command executed", "type", commandType, "status", result.Status, "elapsed", time.Since(start))
	return result
}

func execLaunchctl(action string) *Result {
	out, err := exec.Command("launchctl", action, serviceName).CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return &Result{
			Status:       3,
			Output:       output,
			ErrorMessage: err.Error(),
		}
	}
	if output == "" {
		output = fmt.Sprintf("launchctl %s %s: ok", action, serviceName)
	}
	return &Result{Status: 2, Output: output}
}

func execRestart() *Result {
	// stop then start
	stopOut, _ := exec.Command("launchctl", "stop", serviceName).CombinedOutput()
	time.Sleep(2 * time.Second)
	startOut, err := exec.Command("launchctl", "start", serviceName).CombinedOutput()

	output := fmt.Sprintf("stop: %s\nstart: %s",
		strings.TrimSpace(string(stopOut)),
		strings.TrimSpace(string(startOut)))

	if err != nil {
		return &Result{
			Status:       3,
			Output:       output,
			ErrorMessage: fmt.Sprintf("restart failed on start: %v", err),
		}
	}
	return &Result{Status: 2, Output: output}
}

func execStatus() *Result {
	out, err := exec.Command("launchctl", "list").CombinedOutput()
	if err != nil {
		return &Result{
			Status:       3,
			ErrorMessage: fmt.Sprintf("launchctl list failed: %v", err),
		}
	}

	lines := strings.Split(string(out), "\n")
	var matched string
	for _, line := range lines {
		if strings.Contains(line, serviceName) {
			matched = strings.TrimSpace(line)
			break
		}
	}

	if matched == "" {
		return &Result{Status: 2, Output: fmt.Sprintf("%s: not loaded", serviceName)}
	}

	parts := strings.Fields(matched)
	status := "unknown"
	if len(parts) >= 2 {
		pid := parts[0]
		exitCode := parts[1]
		if pid == "-" {
			status = fmt.Sprintf("not running (last exit: %s)", exitCode)
		} else {
			status = fmt.Sprintf("running (pid: %s)", pid)
		}
	}

	return &Result{Status: 2, Output: fmt.Sprintf("%s: %s", serviceName, status)}
}

func execConfig(params map[string]interface{}) *Result {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".openclaw", "openclaw.json")

	action, _ := params["action"].(string)
	if action == "" {
		action = "read"
	}

	switch action {
	case "read":
		data, err := os.ReadFile(configPath)
		if err != nil {
			return &Result{
				Status:       3,
				ErrorMessage: fmt.Sprintf("read config failed: %v", err),
			}
		}
		// 验证 JSON 合法性
		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return &Result{
				Status:       3,
				Output:       string(data),
				ErrorMessage: fmt.Sprintf("invalid json: %v", err),
			}
		}
		return &Result{Status: 2, Output: string(data)}

	case "write":
		content, _ := params["content"].(string)
		if content == "" {
			return &Result{Status: 3, ErrorMessage: "content is required for write action"}
		}
		// 验证 JSON
		var parsed interface{}
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("invalid json content: %v", err)}
		}
		// 先备份
		if data, err := os.ReadFile(configPath); err == nil {
			backupPath := configPath + ".bak." + time.Now().Format("20060102-150405")
			_ = os.WriteFile(backupPath, data, 0644)
		}
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("mkdir failed: %v", err)}
		}
		// 格式化写入
		formatted, _ := json.MarshalIndent(parsed, "", "  ")
		if err := os.WriteFile(configPath, formatted, 0644); err != nil {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("write config failed: %v", err)}
		}
		return &Result{Status: 2, Output: "config updated successfully"}

	case "validate":
		data, err := os.ReadFile(configPath)
		if err != nil {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("read config failed: %v", err)}
		}
		var parsed interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return &Result{Status: 3, Output: string(data), ErrorMessage: fmt.Sprintf("invalid json: %v", err)}
		}
		return &Result{Status: 2, Output: "config is valid json"}

	default:
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("unknown config action: %s", action)}
	}
}
