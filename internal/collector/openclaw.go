package collector

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"path/filepath"
	"monitor-agent/internal/openclawcli"
	"regexp"
	"sort"
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
	Update     *OpenClawUpdate     `json:"update,omitempty"`
	Gateway    *OpenClawGateway    `json:"gateway,omitempty"`
	Models     *OpenClawModels     `json:"models,omitempty"`
	Daemon     *OpenClawDaemon     `json:"daemon,omitempty"`
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
	ID            string `json:"id"`
	Name          string `json:"name"`
	Sessions      int    `json:"sessions,omitempty"`
	Active        string `json:"active,omitempty"`
	Bootstrap     string `json:"bootstrap,omitempty"`
	SessionModel  string `json:"session_model,omitempty"`
	SessionTokens string `json:"session_tokens,omitempty"`
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

type OpenClawUpdate struct {
	Install   string `json:"install,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Available string `json:"available,omitempty"`
}

type OpenClawGateway struct {
	Service  string `json:"service,omitempty"`
	Runtime  string `json:"runtime,omitempty"`
	Bind     string `json:"bind,omitempty"`
	Port     string `json:"port,omitempty"`
	RPCProbe string `json:"rpc_probe,omitempty"`
}

type OpenClawModels struct {
	Default       string `json:"default,omitempty"`
	ImageModel    string `json:"image_model,omitempty"`
	FallbackCount int    `json:"fallback_count,omitempty"`
	AliasCount    int    `json:"alias_count,omitempty"`
}

type OpenClawDaemon struct {
	Service string `json:"service,omitempty"`
	Runtime string `json:"runtime,omitempty"`
	LogFile string `json:"log_file,omitempty"`
}

// CollectOpenClawInfo 执行 openclaw status --all 并解析输出，用于心跳上报。
// 同时采集 update status、gateway status、models status、daemon status 的精简信息。
func CollectOpenClawInfo() (*OpenClawInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := openclawcli.CommandContext(ctx, "status", "--all")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	out := stdout.String()
	stderrStr := stderr.String()
	if len(stderrStr) > 0 {
		if len(out) > 0 {
			out = out + "\n" + stderrStr
		} else {
			out = stderrStr
		}
	}
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	out = stripANSI(out)
	info, err := parseOpenClawStatusAll(out)
	if err != nil {
		return nil, err
	}
	needsAgents := len(info.Agents) == 0
	needsNames := false
	if !needsAgents {
		for _, a := range info.Agents {
			if strings.TrimSpace(a.Name) == "" {
				needsNames = true
				break
			}
		}
	}
	if needsAgents || needsNames {
		if agents := collectAgentsList(); len(agents) > 0 {
			if needsAgents {
				info.Agents = agents
			} else {
				fallbackByID := make(map[string]OpenClawAgent, len(agents))
				for _, a := range agents {
					if a.ID != "" {
						fallbackByID[a.ID] = a
					}
				}
				for i := range info.Agents {
					fallback, ok := fallbackByID[info.Agents[i].ID]
					if !ok {
						continue
					}
					if strings.TrimSpace(info.Agents[i].Name) == "" && fallback.Name != "" {
						info.Agents[i].Name = fallback.Name
					}
					if info.Agents[i].Sessions == 0 && fallback.Sessions > 0 {
						info.Agents[i].Sessions = fallback.Sessions
					}
					if strings.TrimSpace(info.Agents[i].Active) == "" && fallback.Active != "" {
						info.Agents[i].Active = fallback.Active
					}
					if strings.TrimSpace(info.Agents[i].Bootstrap) == "" && fallback.Bootstrap != "" {
						info.Agents[i].Bootstrap = fallback.Bootstrap
					}
				}
			}
		}
	}

	type collector struct {
		name string
		fn   func() interface{}
	}
	extras := []collector{
		{"update", func() interface{} { return collectUpdateStatus() }},
		{"gateway", func() interface{} { return collectGatewayStatus() }},
		{"models", func() interface{} { return collectModelsStatus() }},
		{"daemon", func() interface{} { return collectDaemonStatus() }},
	}
	type indexedResult struct {
		idx int
		val interface{}
	}
	ch := make(chan indexedResult, len(extras))
	for i, c := range extras {
		go func(idx int, fn func() interface{}) {
			ch <- indexedResult{idx, fn()}
		}(i, c.fn)
	}
	results := make([]interface{}, len(extras))
	for range extras {
		r := <-ch
		results[r.idx] = r.val
	}
	if v, ok := results[0].(*OpenClawUpdate); ok {
		info.Update = v
	}
	if v, ok := results[1].(*OpenClawGateway); ok {
		info.Gateway = v
	}
	if v, ok := results[2].(*OpenClawModels); ok {
		info.Models = v
	}
	if v, ok := results[3].(*OpenClawDaemon); ok {
		info.Daemon = v
	}

	return info, nil
}

func runQuickCLI(timeout time.Duration, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := openclawcli.CommandContext(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	out := stdout.String()
	if se := stderr.String(); se != "" {
		if out != "" {
			out += "\n" + se
		} else {
			out = se
		}
	}
	return stripANSI(strings.TrimSpace(out))
}

func collectAgentsList() []OpenClawAgent {
	out := runQuickCLI(10*time.Second, "agents", "list", "--json")
	if out == "" {
		return nil
	}
	type agentItem struct {
		ID        string      `json:"id"`
		Name      string      `json:"name"`
		Sessions  interface{} `json:"sessions"`
		Active    string      `json:"active"`
		Bootstrap string      `json:"bootstrap"`
	}
	var items []agentItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil
	}
	agents := make([]OpenClawAgent, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = id
		}
		agents = append(agents, OpenClawAgent{
			ID:        id,
			Name:      name,
			Sessions:  parseSessionsValue(item.Sessions),
			Active:    normalizeAgentActive(item.Active),
			Bootstrap: normalizeAgentBootstrap(item.Bootstrap),
		})
	}
	if len(agents) == 0 {
		return nil
	}
	return agents
}

func CollectAgentsFingerprint() string {
	agents := collectAgentsList()
	if len(agents) == 0 {
		return ""
	}
	return AgentsFingerprint(agents)
}

func CollectAgentsOnlyInfo() *OpenClawInfo {
	agents := collectAgentsList()
	if len(agents) == 0 {
		return nil
	}
	return &OpenClawInfo{
		Overview: &OpenClawOverview{
			AgentsSummary: fmt.Sprintf("%d agents", len(agents)),
		},
		Agents:   agents,
		Channels: nil,
		Bindings: nil,
	}
}

func AgentsFingerprint(agents []OpenClawAgent) string {
	if len(agents) == 0 {
		return ""
	}
	type item struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"`
	}
	snapshot := make([]item, 0, len(agents))
	for _, agent := range agents {
		id := strings.TrimSpace(agent.ID)
		if id == "" {
			continue
		}
		snapshot = append(snapshot, item{
			ID:   id,
			Name: strings.TrimSpace(agent.Name),
		})
	}
	if len(snapshot) == 0 {
		return ""
	}
	sort.Slice(snapshot, func(i, j int) bool {
		if snapshot[i].ID == snapshot[j].ID {
			return snapshot[i].Name < snapshot[j].Name
		}
		return snapshot[i].ID < snapshot[j].ID
	})
	data, err := json.Marshal(snapshot)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(data)
	return fmt.Sprintf("%x", sum[:])
}

func collectUpdateStatus() *OpenClawUpdate {
	out := runQuickCLI(10*time.Second, "update", "status")
	if out == "" {
		return nil
	}
	u := &OpenClawUpdate{}
	for _, line := range strings.Split(out, "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(lower, "install") {
			u.Install = extractTableValue(line)
		} else if strings.Contains(lower, "channel") {
			u.Channel = extractTableValue(line)
		} else if strings.Contains(lower, "update") && !strings.HasPrefix(lower, "openclaw") {
			u.Available = extractTableValue(line)
		}
	}
	if u.Install == "" && u.Channel == "" && u.Available == "" {
		if strings.Contains(strings.ToLower(out), "available") {
			u.Available = out
		} else {
			return nil
		}
	}
	return u
}

func collectGatewayStatus() *OpenClawGateway {
	out := runQuickCLI(10*time.Second, "gateway", "status")
	if out == "" {
		return nil
	}
	g := &OpenClawGateway{}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "service:"):
			g.Service = strings.TrimSpace(trimmed[len("Service:"):])
		case strings.HasPrefix(lower, "runtime:"):
			g.Runtime = strings.TrimSpace(trimmed[len("Runtime:"):])
		case strings.HasPrefix(lower, "gateway:") && strings.Contains(lower, "bind"):
			g.Bind = strings.TrimSpace(trimmed[len("Gateway:"):])
		case strings.HasPrefix(lower, "listening:"):
			g.Port = strings.TrimSpace(trimmed[len("Listening:"):])
		case strings.HasPrefix(lower, "rpc probe:"):
			g.RPCProbe = strings.TrimSpace(trimmed[len("RPC probe:"):])
		}
	}
	if g.Service == "" && g.Runtime == "" {
		return nil
	}
	return g
}

func collectModelsStatus() *OpenClawModels {
	out := runQuickCLI(10*time.Second, "models", "status")
	if out == "" {
		return nil
	}
	m := &OpenClawModels{}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "default"):
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				m.Default = strings.TrimSpace(parts[1])
			}
		case strings.HasPrefix(lower, "image model"):
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				m.ImageModel = strings.TrimSpace(parts[1])
			}
		case strings.HasPrefix(lower, "fallbacks"):
			m.FallbackCount = extractParenNumber(trimmed)
		case strings.HasPrefix(lower, "aliases"):
			m.AliasCount = extractParenNumber(trimmed)
		}
	}
	if m.Default == "" {
		return nil
	}
	return m
}

func collectDaemonStatus() *OpenClawDaemon {
	out := runQuickCLI(10*time.Second, "daemon", "status")
	if out == "" {
		return nil
	}
	d := &OpenClawDaemon{}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "service:"):
			d.Service = strings.TrimSpace(trimmed[len("Service:"):])
		case strings.HasPrefix(lower, "runtime:"):
			d.Runtime = strings.TrimSpace(trimmed[len("Runtime:"):])
		case strings.HasPrefix(lower, "file logs:"):
			d.LogFile = strings.TrimSpace(trimmed[len("File logs:"):])
		}
	}
	if d.Service == "" && d.Runtime == "" {
		return nil
	}
	return d
}

func extractTableValue(line string) string {
	for _, sep := range []string{"│", "|"} {
		if strings.Contains(line, sep) {
			parts := strings.Split(line, sep)
			if len(parts) >= 3 {
				return strings.TrimSpace(parts[2])
			}
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	if idx := strings.Index(line, ":"); idx >= 0 {
		return strings.TrimSpace(line[idx+1:])
	}
	return strings.TrimSpace(line)
}

func extractParenNumber(s string) int {
	start := strings.Index(s, "(")
	end := strings.Index(s, ")")
	if start >= 0 && end > start {
		return parseInt(s[start+1:end], 0)
	}
	return 0
}

// stripANSI 去掉 ANSI 转义序列（如 \x1b[32m），便于按纯文本匹配 section 标题
func stripANSI(s string) string {
	// CSI: ESC [ 后跟参数和结尾字母；常见 \x1b[0m \x1b[36m 等
	ansiCSI := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return ansiCSI.ReplaceAllString(s, "")
}

// parseOpenClawStatusAll 解析 openclaw status --all 的完整输出
func parseOpenClawStatusAll(out string) (*OpenClawInfo, error) {
	info := &OpenClawInfo{
		Agents:   nil,
		Channels: nil,
		Bindings: nil,
	}
	lines := strings.Split(out, "\n")

	// Overview 表：Item | Value（部分 CLI 用 "Status" 作为标题）
	if overview := parseOverviewTable(lines, "Overview"); overview != nil {
		info.Overview = overview
	} else if overview := parseOverviewTable(lines, "Status"); overview != nil {
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

	// Agents 表：兼容 Bootstrap/Bootstrap file、Active 等表头变体
	info.Agents = parseAgentsTable(lines)
	if len(info.Agents) == 0 {
		info.Agents = parseAgentsLoose(lines)
	}
	annotateAgentsWithSessions(lines, info.Agents)

	// Diagnosis：Skills: N eligible · M missing
	if d := parseDiagnosis(lines); d != nil {
		info.Diagnosis = d
	}

	return info, nil
}

type openClawSessionSnapshot struct {
	AgentID string
	Key     string
	Model   string
	Tokens  string
}

func annotateAgentsWithSessions(lines []string, agents []OpenClawAgent) {
	if len(agents) == 0 {
		return
	}
	sessions := parseSessionsTable(lines)
	if len(sessions) == 0 {
		sessions = collectSessionsJSON()
	}
	if len(sessions) == 0 {
		return
	}

	bestByAgent := make(map[string]openClawSessionSnapshot, len(sessions))
	for _, session := range sessions {
		if session.AgentID == "" {
			continue
		}
		existing, ok := bestByAgent[session.AgentID]
		if !ok || preferSessionSnapshot(session, existing) {
			bestByAgent[session.AgentID] = session
		}
	}

	for i := range agents {
		s, ok := bestByAgent[agents[i].ID]
		if !ok {
			continue
		}
		if agents[i].SessionModel == "" {
			agents[i].SessionModel = strings.TrimSpace(s.Model)
		}
		if agents[i].SessionTokens == "" {
			agents[i].SessionTokens = strings.TrimSpace(s.Tokens)
		}
	}
}

func preferSessionSnapshot(candidate, existing openClawSessionSnapshot) bool {
	candidateMain := strings.HasSuffix(candidate.Key, ":main")
	existingMain := strings.HasSuffix(existing.Key, ":main")
	if candidateMain != existingMain {
		return candidateMain
	}
	return existing.Key == ""
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
		// 表格格式为 │ Item │ Value │，split 后 cols[0] 常为空，cols[1]=Item, cols[2]=Value（Value 中可能含 │，需拼接）
		if len(cols) >= 3 {
			key := strings.TrimSpace(cols[1])
			if key == "" || key == "Item" || key == "Key" {
				continue
			}
			val := strings.TrimSpace(strings.Join(cols[2:], tableColumnSep))
			merged[key] = val
		} else if len(cols) >= 2 {
			key := strings.TrimSpace(cols[0])
			if key == "" || key == "Item" || key == "Key" {
				continue
			}
			val := strings.TrimSpace(cols[1])
			merged[key] = val
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

// parseAgentsLoose 尝试在表头无法识别时，直接从 Agents section 中提取
func parseAgentsLoose(lines []string) []OpenClawAgent {
	start := findSectionStart(lines, "Agents")
	if start < 0 {
		return nil
	}
	var agents []OpenClawAgent
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if isNextSection(line) {
			break
		}
		if isTableSeparator(line) || strings.TrimSpace(line) == "" {
			continue
		}
		cols := splitTableRow(line)
		if len(cols) == 0 {
			continue
		}
		if isAgentHeaderRow(cols) {
			continue
		}
		offset := 0
		for offset < len(cols) && strings.TrimSpace(cols[offset]) == "" {
			offset++
		}
		if offset >= len(cols) {
			continue
		}
		agentCell := strings.TrimSpace(cols[offset])
		if agentCell == "" {
			continue
		}
		id, name := parseAgentIDAndName(agentCell)
		bootstrap := ""
		sessions := 0
		active := ""
		if offset+1 < len(cols) {
			bootstrap = normalizeAgentBootstrap(cols[offset+1])
		}
		if offset+2 < len(cols) {
			sessions = parseInt(cols[offset+2], 0)
		}
		if offset+3 < len(cols) {
			active = normalizeAgentActive(cols[offset+3])
		}
		agents = append(agents, OpenClawAgent{
			ID:        id,
			Name:      name,
			Sessions:  sessions,
			Active:    active,
			Bootstrap: bootstrap,
		})
	}
	if len(agents) == 0 {
		return nil
	}
	return agents
}

func parseAgentsTable(lines []string) []OpenClawAgent {
	start := findSectionStart(lines, "Agents")
	if start < 0 {
		return nil
	}

	headerIdx := -1
	var headerCols []string
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || isTableSeparator(line) {
			continue
		}
		if isNextSection(lines[i]) && i != start {
			return nil
		}
		cols := splitTableRow(lines[i])
		if isAgentHeaderRow(cols) {
			headerIdx = i
			headerCols = cols
			break
		}
	}
	if headerIdx < 0 {
		return nil
	}

	colIndex := func(names ...string) int {
		for idx, col := range headerCols {
			lower := strings.ToLower(strings.TrimSpace(col))
			for _, name := range names {
				if lower == name {
					return idx
				}
			}
		}
		return -1
	}

	agentCol := colIndex("agent", "agents")
	bootstrapCol := colIndex("bootstrap file", "bootstrap")
	sessionsCol := colIndex("sessions", "session")
	activeCol := colIndex("active", "activity", "last active")
	if agentCol < 0 {
		return nil
	}

	var agents []OpenClawAgent
	for i := headerIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" || isTableSeparator(line) {
			continue
		}
		if isNextSection(line) {
			break
		}
		cols := splitTableRow(line)
		if agentCol >= len(cols) {
			continue
		}
		agentCell := strings.TrimSpace(cols[agentCol])
		if agentCell == "" {
			continue
		}
		id, name := parseAgentIDAndName(agentCell)
		agent := OpenClawAgent{
			ID:   id,
			Name: name,
		}
		if bootstrapCol >= 0 && bootstrapCol < len(cols) {
			agent.Bootstrap = normalizeAgentBootstrap(cols[bootstrapCol])
		}
		if sessionsCol >= 0 && sessionsCol < len(cols) {
			agent.Sessions = parseInt(cols[sessionsCol], 0)
		}
		if activeCol >= 0 && activeCol < len(cols) {
			agent.Active = normalizeAgentActive(cols[activeCol])
		}
		agents = append(agents, agent)
	}
	if len(agents) == 0 {
		return nil
	}
	return agents
}

func parseSessionsTable(lines []string) []openClawSessionSnapshot {
	rows := parseTable(lines, "Sessions", []string{"Key", "Kind", "Age", "Model", "Tokens"})
	if len(rows) == 0 {
		return nil
	}

	var sessions []openClawSessionSnapshot
	for _, row := range rows {
		key := strings.TrimSpace(row["Key"])
		agentID := parseAgentIDFromSessionKey(key)
		if agentID == "" {
			continue
		}
		sessions = append(sessions, openClawSessionSnapshot{
			AgentID: agentID,
			Key:     key,
			Model:   strings.TrimSpace(row["Model"]),
			Tokens:  normalizeSessionTokens(row["Tokens"]),
		})
	}
	return sessions
}

func collectSessionsJSON() []openClawSessionSnapshot {
	out := runQuickCLI(10*time.Second, "sessions", "--all-agents", "--json")
	if out == "" {
		return nil
	}
	var payload struct {
		Sessions []struct {
			Key           string `json:"key"`
			AgentID       string `json:"agentId"`
			Model         string `json:"model"`
			TotalTokens   int64  `json:"totalTokens"`
			ContextTokens int64  `json:"contextTokens"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return nil
	}

	sessions := make([]openClawSessionSnapshot, 0, len(payload.Sessions))
	for _, item := range payload.Sessions {
		agentID := strings.TrimSpace(item.AgentID)
		if agentID == "" {
			agentID = parseAgentIDFromSessionKey(item.Key)
		}
		if agentID == "" {
			continue
		}
		sessions = append(sessions, openClawSessionSnapshot{
			AgentID: agentID,
			Key:     strings.TrimSpace(item.Key),
			Model:   strings.TrimSpace(item.Model),
			Tokens:  formatSessionTokens(item.TotalTokens, item.ContextTokens),
		})
	}
	if len(sessions) == 0 {
		return nil
	}
	return sessions
}

func parseAgentIDFromSessionKey(key string) string {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, "agent:") {
		return ""
	}
	rest := strings.TrimPrefix(key, "agent:")
	parts := strings.Split(rest, ":")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func normalizeSessionTokens(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	if lower := strings.ToLower(value); lower == "unknown" || strings.HasPrefix(lower, "unknown/") {
		return ""
	}
	return value
}

func formatSessionTokens(total, context int64) string {
	switch {
	case total <= 0 && context <= 0:
		return ""
	case context > 0 && total > 0:
		return fmt.Sprintf("%s / %s", formatCompactTokenCount(total), formatCompactTokenCount(context))
	case total > 0:
		return formatCompactTokenCount(total)
	default:
		return formatCompactTokenCount(context)
	}
}

func formatCompactTokenCount(v int64) string {
	if v >= 1_000_000 {
		return fmt.Sprintf("%.1fm", float64(v)/1_000_000)
	}
	if v >= 1_000 {
		f := float64(v) / 1_000
		if v%1_000 == 0 {
			return fmt.Sprintf("%dk", v/1_000)
		}
		return fmt.Sprintf("%.1fk", f)
	}
	return fmt.Sprintf("%d", v)
}

func isAgentHeaderRow(cols []string) bool {
	for _, c := range cols {
		cell := strings.TrimSpace(c)
		if cell == "" {
			continue
		}
		lower := strings.ToLower(cell)
		if lower == "agent" || lower == "agents" || strings.Contains(lower, "bootstrap") || lower == "sessions" || lower == "active" || lower == "store" {
			return true
		}
	}
	return false
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
	// 兼容 "✓ Skills: 18 eligible · 0 missing · /path" 及不同 Unicode 中点
	skillsRe := regexp.MustCompile(`(?i)Skills:\s*(\d+)\s*eligible\s*.*?(\d+)\s*missing`)
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
	title = strings.TrimSpace(title)
	titleLower := strings.ToLower(title)
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == title || strings.ToLower(t) == titleLower {
			return i
		}
		// 兼容标题后带冒号或空格
		if len(title) > 0 && len(t) >= len(title) && strings.ToLower(t[:len(title)]) == titleLower {
			rest := t[len(title):]
			if rest == "" || rest == " " || rest == ":" || strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, ":") {
				return i
			}
		}
	}
	return -1
}

func isTableSeparator(line string) bool {
	if strings.Contains(line, "─") || strings.Contains(line, "├") || strings.Contains(line, "┼") || strings.Contains(line, "└") || strings.Contains(line, "┌") || strings.Contains(line, "┐") {
		return true
	}
	// 非 TTY 时常用 ASCII：仅含 - + | 空格的整行为分隔行
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r != '-' && r != '+' && r != '|' && r != ' ' {
			return false
		}
	}
	return strings.Contains(trimmed, "-") || strings.Contains(trimmed, "|")
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

// splitTableRow 按表格列分割：非 TTY 时 CLI 常输出 ASCII "|"，TTY 为 Unicode "│"
func splitTableRow(line string) []string {
	sep := tableColumnSep
	if !strings.Contains(line, "│") && strings.Contains(line, "|") {
		sep = "|"
	}
	parts := strings.Split(line, sep)
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

func parseSessionsValue(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		return parseInt(val, 0)
	default:
		return 0
	}
}

func normalizeAgentActive(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	lower := strings.ToLower(value)
	switch lower {
	case "-", "—", "n/a", "na", "none", "null", "offline", "inactive", "unknown":
		return ""
	case "just now", "now":
		return "now"
	}

	lower = strings.TrimSuffix(lower, " ago")
	lower = strings.Join(strings.Fields(lower), " ")

	if matched, _ := regexp.MatchString(`^\d+[smhdw]$`, lower); matched {
		return lower
	}
	if matched, _ := regexp.MatchString(`^\d+\s*[smhdw]$`, lower); matched {
		return strings.ReplaceAll(lower, " ", "")
	}
	return lower
}

func normalizeAgentBootstrap(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	switch lower {
	case "-", "—", "n/a", "na", "none", "null", "default", "unknown":
		return ""
	}

	value = strings.Trim(value, "\"'")
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		base := filepath.Base(value)
		if base != "." && base != "/" && base != "" {
			return base
		}
	}
	return value
}
