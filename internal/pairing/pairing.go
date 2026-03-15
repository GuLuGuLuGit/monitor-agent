package pairing

import (
	"encoding/json"
	"fmt"
	"time"

	"monitor-agent/internal/identity"
	"monitor-agent/pkg/client"
	agentCrypto "monitor-agent/pkg/crypto"
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
	Status         string                `json:"status"`
	PairingCode    string                `json:"pairing_code"`
	ExpiresIn      int                   `json:"expires_in"`
	APIKeyEnvelope *agentCrypto.Envelope `json:"api_key_envelope"`
}

type PairingCodeInfo struct {
	NodeID      string    `json:"node_id"`
	Hostname    string    `json:"hostname,omitempty"`
	PairingCode string    `json:"pairing_code"`
	ExpiresIn   int       `json:"expires_in"`
	ExpiresAt   time.Time `json:"expires_at"`
	Status      string    `json:"status"`
}

type PairingStatusInfo struct {
	NodeID         string                `json:"node_id"`
	Status         string                `json:"status"`
	PairingCode    string                `json:"pairing_code,omitempty"`
	ExpiresIn      int                   `json:"expires_in,omitempty"`
	ExpiresAt      time.Time             `json:"expires_at,omitempty"`
	APIKeyEnvelope *agentCrypto.Envelope `json:"api_key_envelope,omitempty"`
}

func RequestPairingCode(httpClient *client.Client, id *identity.Identity, hostname, osVersion string) (*PairingCodeInfo, error) {
	pubPEM, err := id.PublicKeyPEM()
	if err != nil {
		return nil, fmt.Errorf("get public key PEM: %w", err)
	}

	req := pairingRequest{
		NodeID:       id.NodeID,
		PublicKeyPEM: pubPEM,
		Hostname:     hostname,
		OSVersion:    osVersion,
	}
	var resp pairingRequestResp
	if err := httpClient.Post(requestPath, req, &resp); err != nil {
		return nil, fmt.Errorf("request pairing code: %w", err)
	}

	return &PairingCodeInfo{
		NodeID:      id.NodeID,
		Hostname:    hostname,
		PairingCode: resp.PairingCode,
		ExpiresIn:   resp.ExpiresIn,
		ExpiresAt:   time.Now().UTC().Add(time.Duration(resp.ExpiresIn) * time.Second),
		Status:      "pending",
	}, nil
}

func GetPairingStatus(httpClient *client.Client, nodeID string) (*PairingStatusInfo, error) {
	var statusResp pairingStatusResp
	statusURL := fmt.Sprintf("%s?node_id=%s", statusPath, nodeID)
	if err := httpClient.Get(statusURL, &statusResp); err != nil {
		return nil, fmt.Errorf("get pairing status: %w", err)
	}
	return &PairingStatusInfo{
		NodeID:         nodeID,
		Status:         statusResp.Status,
		PairingCode:    statusResp.PairingCode,
		ExpiresIn:      statusResp.ExpiresIn,
		ExpiresAt:      time.Now().UTC().Add(time.Duration(statusResp.ExpiresIn) * time.Second),
		APIKeyEnvelope: statusResp.APIKeyEnvelope,
	}, nil
}

// RunPairing executes the pairing flow: request code, display it, poll until
// confirmed or expired. Returns the API key on success.
func RunPairing(httpClient *client.Client, id *identity.Identity, hostname, osVersion string) (string, error) {
	for {
		codeInfo, err := RequestPairingCode(httpClient, id, hostname, osVersion)
		if err != nil {
			return "", err
		}

		// 2. Display pairing code
		fmt.Println()
		fmt.Println("╔═══════════════════════════════════════════╗")
		fmt.Println("║          OpenClaw 设备配对                ║")
		fmt.Println("╠═══════════════════════════════════════════╣")
		fmt.Printf("║                                           ║\n")
		fmt.Printf("║     配对码:  %s                       ║\n", codeInfo.PairingCode)
		fmt.Printf("║                                           ║\n")
		fmt.Printf("║     请在 Web 管理端输入此配对码            ║\n")
		fmt.Printf("║     有效期: %d 秒                         ║\n", codeInfo.ExpiresIn)
		fmt.Println("║                                           ║")
		fmt.Println("╚═══════════════════════════════════════════╝")
		fmt.Println()

		logger.Info("pairing code displayed", "code", codeInfo.PairingCode, "expires_in", codeInfo.ExpiresIn)

		// 3. Poll for confirmation
		deadline := time.Now().Add(time.Duration(codeInfo.ExpiresIn) * time.Second)
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
				if statusResp.APIKeyEnvelope == nil {
					return "", fmt.Errorf("pairing completed but api key envelope missing")
				}
				plaintext, err := agentCrypto.Open(id.PrivateKey, statusResp.APIKeyEnvelope)
				if err != nil {
					return "", fmt.Errorf("decrypt api key envelope: %w", err)
				}
				var payload struct {
					APIKey string `json:"api_key"`
				}
				if err := json.Unmarshal(plaintext, &payload); err != nil {
					return "", fmt.Errorf("decode api key envelope: %w", err)
				}
				if payload.APIKey == "" {
					return "", fmt.Errorf("pairing completed but api key missing")
				}
				logger.Info("pairing successful")
				fmt.Println("[✓] 配对成功！设备已绑定。")
				return payload.APIKey, nil
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
