package pairing

import (
	"fmt"
	"time"

	"monitor-agent/internal/identity"
	"monitor-agent/pkg/client"
	"monitor-agent/pkg/logger"
)

const (
	requestPath = "/api/v1/agent/pairing/request"
	statusPath  = "/api/v1/agent/pairing/status"

	pollInterval = 3 * time.Second
)

type pairingRequest struct {
	NodeID       string `json:"node_id"`
	PublicKeyPEM string `json:"public_key_pem"`
	Hostname     string `json:"hostname"`
	OSVersion    string `json:"os_version"`
}

type pairingRequestResp struct {
	PairingCode string `json:"pairing_code"`
	ExpiresIn   int    `json:"expires_in"`
}

type pairingStatusResp struct {
	Status string `json:"status"`
	APIKey string `json:"api_key"`
}

// RunPairing executes the pairing flow: request code, display it, poll until
// confirmed or expired. Returns the API key on success.
func RunPairing(httpClient *client.Client, id *identity.Identity, hostname, osVersion string) (string, error) {
	pubPEM, err := id.PublicKeyPEM()
	if err != nil {
		return "", fmt.Errorf("get public key PEM: %w", err)
	}

	for {
		// 1. Request pairing code
		req := pairingRequest{
			NodeID:       id.NodeID,
			PublicKeyPEM: pubPEM,
			Hostname:     hostname,
			OSVersion:    osVersion,
		}
		var resp pairingRequestResp
		if err := httpClient.Post(requestPath, req, &resp); err != nil {
			return "", fmt.Errorf("request pairing code: %w", err)
		}

		// 2. Display pairing code
		fmt.Println()
		fmt.Println("╔═══════════════════════════════════════════╗")
		fmt.Println("║          OpenClaw 设备配对                ║")
		fmt.Println("╠═══════════════════════════════════════════╣")
		fmt.Printf("║                                           ║\n")
		fmt.Printf("║     配对码:  %s                       ║\n", resp.PairingCode)
		fmt.Printf("║                                           ║\n")
		fmt.Printf("║     请在 Web 管理端输入此配对码            ║\n")
		fmt.Printf("║     有效期: %d 秒                         ║\n", resp.ExpiresIn)
		fmt.Println("║                                           ║")
		fmt.Println("╚═══════════════════════════════════════════╝")
		fmt.Println()

		logger.Info("pairing code displayed", "code", resp.PairingCode, "expires_in", resp.ExpiresIn)

		// 3. Poll for confirmation
		deadline := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(pollInterval)

			var statusResp pairingStatusResp
			statusURL := fmt.Sprintf("%s?node_id=%s", statusPath, id.NodeID)
			if err := httpClient.Get(statusURL, &statusResp); err != nil {
				logger.Warn("poll pairing status failed", "err", err)
				continue
			}

			switch statusResp.Status {
			case "paired":
				logger.Info("pairing successful")
				fmt.Println("[✓] 配对成功！设备已绑定。")
				return statusResp.APIKey, nil
			case "expired":
				logger.Info("pairing code expired, requesting new code")
				fmt.Println("[!] 配对码已过期，正在申请新的配对码...")
				break
			case "pending":
				// keep polling
			}

			if statusResp.Status == "expired" {
				break
			}
		}
	}
}
