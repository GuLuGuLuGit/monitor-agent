package collector

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// 表格行按 Unicode 竖线分割（openclaw status --all 输出）
const tableColumnSep = "│"

// OpenClawInfo 与前端 OpenClawInfo 兼容，并增加 overview/diagnosis
type OpenClawInfo struct {
	Overview   *OpenClawOverview   `json:"overview,omitempty"`
	Agents     []OpenClawAgent     `json:"agents"`
	Channels   []OpenClawChannel   `json:"channels"`
	Bindings   []OpenClawBinding   `json:"bindings"`
	Model      string              `json:"model,omitempty"`
	Diagnosis  *OpenClawDiagnosis  `json:"diagnosis,omitempty"`
}

type OpenClawOverview struct {
	Version         string `json:"version,omitempty"`
	OS              string `json:"os,omitempty"`
	Node            string `json:"node,omitempty"`
	Config          string `json:"config,omitempty"`
	Dashboard       string `json:"dashboard,omitempty"`
	Tailscale       string `json:"tailscale,omitempty"`
	Channel         string `json:"channel,omitempty"`
	Update          string `json:"update,omitempty"`
	Gateway         string `json:"gateway,omitempty"`
	GatewaySelf     string `json:"gateway_self,omitempty"`
	GatewayService  string `json:"gateway_service,omitempty"`
	NodeService     string `json:"node_service,omitempty"`
	AgentsSummary   string `json:"agents_summary,omitempty"`
}

type OpenClawAgent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Sessions  int    `json:"sessions,omitempty"`
	Active    string `json:"active,omitempty"`
	Bootstrap string `json:"bootstrap,omitempty"`
}

type OpenClawChannel struct {
	Type     string            `json:"type"`
	Enabled  bool              `json:"enabled"`
	State    string            `json:"state,omitempty"`
	Detail   string            `json:"detail,omitempty"`
	Accounts []OpenClawAccount `json:"accounts"`
}

type OpenClawAccount struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status,omitempty"`
}

type OpenClawBinding struct {
	AgentID   string `json:"agent_id"`
	Channel   string `json:"channel"`
	AccountID string `json:"account_id"`
}

type OpenClawDiagnosis struct {
	SkillsEligible int    `json:"skills_eligible,omitempty"`
	SkillsMissing  int    `json:"skills_missing,omitempty"`
	ChannelIssues  string `json:"channel_issues,omitempty"`
}

// CollectOpenClawInfo 执行 openclaw status --all 并解析输出，用于心跳上报
func CollectOpenClawInfo() (*OpenClawInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "openclaw", "status", "--all")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	out := stdout.String()
	return parseOpenClawStatusAll(out)
}

// parseOpenClawStatusAll 解析 openclaw status --all 的完整输出
func parseOpenClawStatusAll(out string) (*OpenClawInfo, error) {
	info := &OpenClawInfo{
		Agents:   nil,
		Channels: nil,
		Bindings: nil,
	}
	lines := strings.Split(out, "\n")

	// Overview 表：Item | Value
	if overview := parseOverviewTable(lines, "Overview"); overview != nil {
		info.Overview = overview
	}

	// Channels 表：Channel | Enabled | State | Detail
	channelRows := parseTable(lines, "Channels", []string{"Channel", "Enabled", "State", "Detail"})
	for _, row := range channelRows {
		ch := OpenClawChannel{
			Type:     strings.TrimSpace(row["Channel"]),
			Enabled:  strings.EqualFold(strings.TrimSpace(row["Enabled"]), "ON"),
			State:    strings.TrimSpace(row["State"]),
			Detail:   strings.TrimSpace(row["Detail"]),
			Accounts: nil,
		}
		if ch.Type == "" {
			continue
		}
		// 查找对应 "X accounts" 表（如 Telegram accounts）
		ch.Accounts = parseChannelAccounts(lines, ch.Type)
		info.Channels = append(info.Channels, ch)
	}

	// Agents 表：Agent | Bootstrap file | Sessions | Active | Store
	agentRows := parseTable(lines, "Agents", []string{"Agent", "Bootstrap file", "Sessions", "Active", "Store"})
	for _, row := range agentRows {
		agentStr := strings.TrimSpace(row["Agent"])
		if agentStr == "" {
			continue
		}
		id, name := parseAgentIDAndName(agentStr)
		sessions := parseInt(row["Sessions"], 0)
		info.Agents = append(info.Agents, OpenClawAgent{
			ID:        id,
			Name:      name,
			Sessions:  sessions,
			Active:    strings.TrimSpace(row["Active"]),
			Bootstrap: strings.TrimSpace(row["Bootstrap file"]),
		})
	}

	// Diagnosis：Skills: N eligible · M missing
	if d := parseDiagnosis(lines); d != nil {
		info.Diagnosis = d
	}

	return info, nil
}

// parseOverviewTable 解析 Overview 这样的 Key-Value 表，返回 overview 结构
func parseOverviewTable(lines []string, sectionTitle string) *OpenClawOverview {
	rows := parseKeyValueTable(lines, sectionTitle)
	if len(rows) == 0 {
		return nil
	}
	get := func(key string) string { return strings.TrimSpace(rows[0][key]) }
	return &OpenClawOverview{
		Version:        get("Version"),
		OS:             get("OS"),
		Node:           get("Node"),
		Config:         get("Config"),
		Dashboard:      get("Dashboard"),
		Tailscale:      get("Tailscale"),
		Channel:        get("Channel"),
		Update:         get("Update"),
		Gateway:        get("Gateway"),
		GatewaySelf:    get("Gateway self"),
		GatewayService: get("Gateway service"),
		NodeService:    get("Node service"),
		AgentsSummary:  get("Agents"),
	}
}

// parseKeyValueTable 解析两列表（Item | Value），返回 []map[string]string，每行一个 map（仅一条 key-value）
// 合并为单 map：key=Item, value=Value
func parseKeyValueTable(lines []string, sectionTitle string) []map[string]string {
	start := findSectionStart(lines, sectionTitle)
	if start < 0 {
		return nil
	}
	var out []map[string]string
	merged := make(map[string]string)
	for i := start; i < len(lines); i++ {
		line := lines[i]
		if isTableSeparator(line) || line == "" {
			continue
		}
		if isNextSection(line) {
			break
		}
		cols := splitTableRow(line)
		// 表格格式为 │ Item │ Value │，split 后 cols[0] 常为空，cols[1]=Item, cols[2]=Value
		if len(cols) >= 3 {
			key := strings.TrimSpace(cols[1])
			val := strings.TrimSpace(cols[2])
			if key != "" && key != "Item" {
				merged[key] = val
			}
		} else if len(cols) >= 2 {
			key := strings.TrimSpace(cols[0])
			val := strings.TrimSpace(cols[1])
			if key != "" && key != "Item" {
				merged[key] = val
			}
		}
	}
	if len(merged) > 0 {
		out = append(out, merged)
	}
	return out
}

// parseTable 解析多列表，表头由 headers 指定，返回每行一个 map
func parseTable(lines []string, sectionTitle string, headers []string) []map[string]string {
	start := findSectionStart(lines, sectionTitle)
	if start < 0 || len(headers) == 0 {
		return nil
	}
	// 定位表头行，并确定第一列在 split 后的起始下标（openclaw 表格常为 │ A │ B │，split 后 [ "", "A", "B" ]）
	var headerIdx, colOffset int = -1, 0
	for i := start; i < len(lines); i++ {
		cols := splitTableRow(lines[i])
		for c := 0; c < len(cols) && c < 3; c++ {
			if strings.TrimSpace(cols[c]) == headers[0] {
				headerIdx = i
				colOffset = c
				break
			}
		}
		if headerIdx >= 0 {
			break
		}
		if isNextSection(lines[i]) {
			return nil
		}
	}
	if headerIdx < 0 {
		return nil
	}
	var result []map[string]string
	for i := headerIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if isTableSeparator(line) || line == "" {
			continue
		}
		if isNextSection(line) {
			break
		}
		cols := splitTableRow(line)
		if colOffset+len(headers) > len(cols) {
			continue
		}
		row := make(map[string]string)
		for j, h := range headers {
			row[h] = cols[colOffset+j]
		}
		first := strings.TrimSpace(row[headers[0]])
		if first == "" {
			continue
		}
		result = append(result, row)
	}
	return result
}

// parseChannelAccounts 查找 "Telegram accounts" 或 "{channel} accounts" 表，返回账号列表
func parseChannelAccounts(lines []string, channelType string) []OpenClawAccount {
	// 表名可能是 "Telegram accounts" 或 "Channels" 后的子表
	sectionName := channelType + " accounts"
	start := findSectionStart(lines, sectionName)
	if start < 0 {
		return nil
	}
	// 表头一般为 Account | Status | Notes
	headers := []string{"Account", "Status", "Notes"}
	rows := parseTableWithHeaderAt(lines, start, headers)
	var accounts []OpenClawAccount
	for _, row := range rows {
		id := strings.TrimSpace(row["Account"])
		if id == "" {
			continue
		}
		status := strings.TrimSpace(row["Status"])
		accounts = append(accounts, OpenClawAccount{
			ID:      id,
			Enabled: strings.EqualFold(status, "OK"),
			Status:  status,
		})
	}
	return accounts
}

// parseTableWithHeaderAt 从 start 行开始找表头（第一个单元格匹配 headers[0]），然后解析数据行
func parseTableWithHeaderAt(lines []string, start int, headers []string) []map[string]string {
	var headerIdx, colOffset int = -1, 0
	for i := start; i < len(lines); i++ {
		cols := splitTableRow(lines[i])
		for c := 0; c < len(cols) && c < 3; c++ {
			if strings.TrimSpace(cols[c]) == headers[0] {
				headerIdx = i
				colOffset = c
				break
			}
		}
		if headerIdx >= 0 {
			break
		}
		if isNextSection(lines[i]) {
			return nil
		}
	}
	if headerIdx < 0 {
		return nil
	}
	var result []map[string]string
	for i := headerIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if isTableSeparator(line) || line == "" {
			continue
		}
		if isNextSection(line) {
			break
		}
		cols := splitTableRow(line)
		if colOffset+len(headers) > len(cols) {
			continue
		}
		row := make(map[string]string)
		for j, h := range headers {
			row[h] = cols[colOffset+j]
		}
		if strings.TrimSpace(row[headers[0]]) == "" {
			continue
		}
		result = append(result, row)
	}
	return result
}

func parseDiagnosis(lines []string) *OpenClawDiagnosis {
	d := &OpenClawDiagnosis{}
	skillsRe := regexp.MustCompile(`Skills:\s*(\d+)\s*eligible\s*·\s*(\d+)\s*missing`)
	issuesRe := regexp.MustCompile(`(?:✓|!)\s*Channel issues:\s*(.+)`)
	for _, line := range lines {
		if m := skillsRe.FindStringSubmatch(line); len(m) >= 3 {
			d.SkillsEligible = parseInt(m[1], 0)
			d.SkillsMissing = parseInt(m[2], 0)
		}
		if m := issuesRe.FindStringSubmatch(line); len(m) >= 2 {
			d.ChannelIssues = strings.TrimSpace(m[1])
		}
	}
	if d.SkillsEligible == 0 && d.SkillsMissing == 0 && d.ChannelIssues == "" {
		return nil
	}
	return d
}

// parseAgentIDAndName 从 "ceo (CEO)" 解析出 id=ceo, name=CEO
func parseAgentIDAndName(s string) (id, name string) {
	s = strings.TrimSpace(s)
	idx := strings.Index(s, " (")
	if idx <= 0 {
		return s, ""
	}
	id = strings.TrimSpace(s[:idx])
	name = strings.TrimSpace(s[idx+2:])
	if len(name) > 0 && name[len(name)-1] == ')' {
		name = strings.TrimSpace(name[:len(name)-1])
	}
	return id, name
}

func findSectionStart(lines []string, title string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) == title {
			return i
		}
	}
	return -1
}

func isTableSeparator(line string) bool {
	return strings.Contains(line, "─") || strings.Contains(line, "├") || strings.Contains(line, "┼") || strings.Contains(line, "└") || strings.Contains(line, "┌") || strings.Contains(line, "┐")
}

func isNextSection(line string) bool {
	trimmed := strings.TrimSpace(line)
	// 下一个 section 通常是单独一行的标题，如 "Channels", "Agents", "Diagnosis"
	if trimmed == "" {
		return false
	}
	sections := []string{"Channels", "Telegram accounts", "Agents", "Diagnosis", "Gateway connection", "Gateway last log", "Pasteable debug", "FAQ:", "Troubleshooting:", "Next steps:"}
	for _, s := range sections {
		if trimmed == s || strings.HasPrefix(trimmed, s) {
			return true
		}
	}
	return false
}

func splitTableRow(line string) []string {
	parts := strings.Split(line, tableColumnSep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseInt(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	var n int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		} else {
			break
		}
	}
	if n == 0 && s != "0" {
		return defaultVal
	}
	return n
}
