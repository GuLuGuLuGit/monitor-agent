package openclawcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	resolvedPath string
	resolveOnce  sync.Once
)

// BinaryPath resolves the openclaw executable path for non-interactive environments.
func BinaryPath() string {
	resolveOnce.Do(func() {
		candidates := make([]string, 0, 6)
		if v := strings.TrimSpace(os.Getenv("OPENCLAW_BIN")); v != "" {
			candidates = append(candidates, v)
		}
		if path, err := exec.LookPath("openclaw"); err == nil && strings.TrimSpace(path) != "" {
			candidates = append(candidates, path)
		}
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			candidates = append(candidates,
				filepath.Join(home, ".npm-global", "bin", "openclaw"),
				filepath.Join(home, ".local", "bin", "openclaw"),
			)
		}
		candidates = append(candidates,
			"/opt/homebrew/bin/openclaw",
			"/usr/local/bin/openclaw",
			"/usr/bin/openclaw",
		)

		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				resolvedPath = candidate
				return
			}
		}

		resolvedPath = "openclaw"
	})
	return resolvedPath
}
