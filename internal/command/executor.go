package command

import (
	"context"
	"encoding/json"
	"fmt"
	"monitor-agent/internal/openclawcli"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"monitor-agent/pkg/logger"
)

const (
	TypeStart    = "openclaw_start"
	TypeStop     = "openclaw_stop"
	TypeRestart  = "openclaw_restart"
	TypeStatus   = "openclaw_status"
	TypeAgents   = "openclaw_agents"
	TypeConfig   = "openclaw_config"
	TypeDoctor   = "openclaw_doctor"
	TypeUpdate   = "openclaw_update"
	TypeLogs     = "openclaw_logs"
	TypeProbe    = "openclaw_probe"
	TypeSessions = "openclaw_sessions"
	TypeSecurity = "openclaw_security"
	TypeGateway  = "openclaw_gateway"
	TypeMessage  = "openclaw_message"

	serviceName    = "ai.openclaw.gateway"
	defaultTimeout = 60 * time.Second
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
	case TypeAgents:
		result = execAgents()
	case TypeConfig:
		result = execConfig(params)
	case TypeDoctor:
		result = execDoctor()
	case TypeUpdate:
		result = execUpdate(params)
	case TypeLogs:
		result = execLogs(params)
	case TypeProbe:
		result = execProbe()
	case TypeSessions:
		result = execSessions(params)
	case TypeSecurity:
		result = execSecurity(params)
	case TypeGateway:
		result = execGatewayCmd(params)
	case TypeMessage:
		result = execMessage(params)
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

// runCLI executes an openclaw CLI command with a timeout and returns combined stdout+stderr.
func runCLI(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := openclawcli.CommandContext(ctx, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func execDoctor() *Result {
	out, err := runCLI(defaultTimeout, "doctor")
	if err != nil && out == "" {
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw doctor failed: %v", err)}
	}
	return &Result{Status: 2, Output: out}
}

func execAgents() *Result {
	out, err := runCLI(15*time.Second, "agents", "list", "--json")
	if err != nil && out == "" {
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw agents list failed: %v", err)}
	}
	return &Result{Status: 2, Output: out}
}

func execUpdate(params map[string]interface{}) *Result {
	action, _ := params["action"].(string)
	if action == "" {
		action = "check"
	}

	switch action {
	case "check":
		out, err := runCLI(30*time.Second, "update", "status")
		if err != nil && out == "" {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw update status failed: %v", err)}
		}
		return &Result{Status: 2, Output: out}
	case "apply":
		channel, _ := params["channel"].(string)
		args := []string{"update"}
		if channel != "" {
			args = append(args, "--channel", channel)
		}
		out, err := runCLI(5*time.Minute, args...)
		if err != nil && out == "" {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw update failed: %v", err)}
		}
		return &Result{Status: 2, Output: out}
	default:
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("unknown update action: %s (use check or apply)", action)}
	}
}

func execLogs(params map[string]interface{}) *Result {
	lines := 50
	if v, ok := params["lines"].(float64); ok && v > 0 {
		lines = int(v)
		if lines > 500 {
			lines = 500
		}
	}

	args := []string{"logs", fmt.Sprintf("--lines=%d", lines)}
	out, err := runCLI(15*time.Second, args...)
	if err != nil && out == "" {
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw logs failed: %v", err)}
	}
	return &Result{Status: 2, Output: out}
}

func execProbe() *Result {
	var parts []string

	gwOut, _ := runCLI(15*time.Second, "gateway", "probe")
	if gwOut != "" {
		parts = append(parts, "=== Gateway Probe ===\n"+gwOut)
	}

	chOut, _ := runCLI(30*time.Second, "channels", "status", "--probe")
	if chOut != "" {
		parts = append(parts, "=== Channels Probe ===\n"+chOut)
	}

	if len(parts) == 0 {
		return &Result{Status: 3, ErrorMessage: "probe commands returned no output"}
	}
	return &Result{Status: 2, Output: strings.Join(parts, "\n\n")}
}

func execSessions(params map[string]interface{}) *Result {
	action, _ := params["action"].(string)
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		out, err := runCLI(15*time.Second, "sessions")
		if err != nil && out == "" {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw sessions failed: %v", err)}
		}
		return &Result{Status: 2, Output: out}
	case "cleanup":
		dryRun, _ := params["dry_run"].(bool)
		args := []string{"sessions", "cleanup"}
		if dryRun {
			args = append(args, "--dry-run")
		}
		out, err := runCLI(30*time.Second, args...)
		if err != nil && out == "" {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw sessions cleanup failed: %v", err)}
		}
		return &Result{Status: 2, Output: out}
	default:
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("unknown sessions action: %s (use list or cleanup)", action)}
	}
}

func execSecurity(params map[string]interface{}) *Result {
	deep, _ := params["deep"].(bool)
	args := []string{"security", "audit"}
	if deep {
		args = append(args, "--deep")
	}
	out, err := runCLI(defaultTimeout, args...)
	if err != nil && out == "" {
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw security audit failed: %v", err)}
	}
	return &Result{Status: 2, Output: out}
}

func execGatewayCmd(params map[string]interface{}) *Result {
	action, _ := params["action"].(string)
	if action == "" {
		action = "status"
	}

	switch action {
	case "status":
		out, err := runCLI(15*time.Second, "gateway", "status")
		if err != nil && out == "" {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw gateway status failed: %v", err)}
		}
		return &Result{Status: 2, Output: out}
	case "health":
		out, err := runCLI(15*time.Second, "gateway", "health")
		if err != nil && out == "" {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw gateway health failed: %v", err)}
		}
		return &Result{Status: 2, Output: out}
	case "restart":
		out, err := runCLI(30*time.Second, "gateway", "restart")
		if err != nil && out == "" {
			return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw gateway restart failed: %v", err)}
		}
		return &Result{Status: 2, Output: out}
	default:
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("unknown gateway action: %s (use status, health, or restart)", action)}
	}
}

func execMessage(params map[string]interface{}) *Result {
	message, _ := params["message"].(string)
	if strings.TrimSpace(message) == "" {
		return &Result{Status: 3, ErrorMessage: "message is required"}
	}

	agentID, _ := params["agent_id"].(string)
	args := []string{"agent", "--message", message, "--json"}
	if agentID != "" {
		args = append(args, "--agent", agentID)
	}

	if v, ok := params["session_id"].(string); ok && v != "" {
		args = append(args, "--session-id", v)
	}
	if v, ok := params["channel"].(string); ok && v != "" {
		args = append(args, "--channel", v)
	}
	if v, ok := params["to"].(string); ok && v != "" {
		args = append(args, "--to", v)
	}
	if v, ok := params["deliver"].(bool); ok && v {
		args = append(args, "--deliver")
	}
	if v, ok := params["thinking"].(string); ok && v != "" {
		args = append(args, "--thinking", v)
	}

	timeout := defaultTimeout
	if v, ok := params["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(int(v)) * time.Second
	}

	out, err := runCLI(timeout, args...)
	if err != nil && out == "" {
		return &Result{Status: 3, ErrorMessage: fmt.Sprintf("openclaw agent failed: %v", err)}
	}

	reply := extractAgentReply(out)
	if err != nil {
		return &Result{Status: 3, Output: reply, ErrorMessage: err.Error()}
	}
	return &Result{Status: 2, Output: reply}
}

func extractAgentReply(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return out
	}

	// Try best-effort JSON extraction first.
	var payload map[string]interface{}
	if json.Unmarshal([]byte(out), &payload) == nil {
		if v := pickFirstString(payload, "reply", "message", "output", "result"); v != "" {
			return v
		}
		if data, ok := payload["data"].(map[string]interface{}); ok {
			if v := pickFirstString(data, "reply", "message", "output", "result"); v != "" {
				return v
			}
		}
	}
	return out
}

func pickFirstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
