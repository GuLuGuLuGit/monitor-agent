package openclawcli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	openclawPath string
	nodePath     string
	resolveOnce  sync.Once
	configPath   string
	configOnce   sync.Once
)

func resolvePaths() {
	resolveOnce.Do(func() {
		openclawPath = resolveExecutable("OPENCLAW_BIN", "openclaw", []string{
			".npm-global/bin/openclaw",
			".local/bin/openclaw",
		}, []string{
			"/opt/homebrew/bin/openclaw",
			"/usr/local/bin/openclaw",
			"/usr/bin/openclaw",
		})
		nodePath = resolveExecutable("OPENCLAW_NODE_BIN", "node", nil, []string{
			"/opt/homebrew/bin/node",
			"/usr/local/bin/node",
			"/usr/bin/node",
		})
	})
}

func resolveExecutable(envKey, binary string, homeRel []string, fixed []string) string {
	candidates := make([]string, 0, 1+len(homeRel)+len(fixed)+1)
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		candidates = append(candidates, v)
	}
	if path, err := exec.LookPath(binary); err == nil && strings.TrimSpace(path) != "" {
		candidates = append(candidates, path)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		for _, rel := range homeRel {
			candidates = append(candidates, filepath.Join(home, rel))
		}
	}
	candidates = append(candidates, fixed...)

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return binary
}

// BinaryPath resolves the openclaw executable path for non-interactive environments.
func BinaryPath() string {
	resolvePaths()
	return openclawPath
}

// NodePath resolves the node executable path for non-interactive environments.
func NodePath() string {
	resolvePaths()
	return nodePath
}

// ConfigPath discovers the active OpenClaw config file path without assuming a fixed location.
func ConfigPath(ctx context.Context) string {
	configOnce.Do(func() {
		if v := strings.TrimSpace(os.Getenv("OPENCLAW_CONFIG_PATH")); v != "" {
			configPath = expandUser(v)
			return
		}
		if v := strings.TrimSpace(os.Getenv("OPENCLAW_CONFIG")); v != "" {
			configPath = expandUser(v)
			return
		}

		out := runStatusAll(ctx)
		if out == "" {
			return
		}
		re := regexp.MustCompile(`(?m)^│\s*Config\s*│\s*(.+?)\s*│\s*$`)
		matches := re.FindStringSubmatch(out)
		if len(matches) < 2 {
			return
		}
		configPath = expandUser(strings.TrimSpace(matches[1]))
	})
	return configPath
}

func runStatusAll(ctx context.Context) string {
	cmd := CommandContext(ctx, "status", "--all")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return string(out)
}

func expandUser(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// CommandContext builds an exec.Cmd that works both for native binaries and env-node scripts.
func CommandContext(ctx context.Context, args ...string) *exec.Cmd {
	bin := BinaryPath()
	commandArgs := append([]string(nil), args...)
	if usesEnvNode(bin) {
		return exec.CommandContext(ctx, NodePath(), append([]string{bin}, commandArgs...)...)
	}
	return exec.CommandContext(ctx, bin, commandArgs...)
}

func DebugSummary() string {
	resolvePaths()
	return fmt.Sprintf("openclaw=%s node=%s", openclawPath, nodePath)
}

func usesEnvNode(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "#!") && strings.Contains(line, "/usr/bin/env node")
}
