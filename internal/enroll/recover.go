package enroll

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"xentz-agent/internal/validation"
)

type RecoverRequest struct {
	RecoveryToken string                 `json:"recovery_token"`
	PrincipalID   string                 `json:"principal_id"`
	DisplayName   string                 `json:"display_name,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type RecoverResponse struct {
	TenantID     string `json:"tenant_id"`
	DeviceID     string `json:"device_id"`
	PrincipalID  string `json:"principal_id"`
	DeviceAPIKey string `json:"device_api_key"`
	RepoPath     string `json:"repo_path"`
	Password     string `json:"password"`
	ControlBase  string `json:"control_base"`
}

func Recover(serverURL, recoveryToken, principalID, displayName string) (*EnrollmentResult, error) {
	if serverURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if strings.TrimSpace(recoveryToken) == "" {
		return nil, fmt.Errorf("recovery token is required")
	}
	if strings.TrimSpace(principalID) == "" {
		return nil, fmt.Errorf("principal ID is required")
	}
	if err := validation.ValidateServerURL(serverURL); err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	meta, _ := GetDeviceMetadata()
	metaMap := map[string]interface{}{
		"hostname": meta.Hostname,
		"os":       meta.OS,
		"arch":     meta.Arch,
	}

	reqBody := RecoverRequest{
		RecoveryToken: strings.TrimSpace(recoveryToken),
		PrincipalID:   strings.TrimSpace(principalID),
		DisplayName:   strings.TrimSpace(displayName),
		Metadata:      metaMap,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal recover request: %w", err)
	}

	url := fmt.Sprintf("%s/control/v1/recover", serverURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("recover request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errMsg bytes.Buffer
		errMsg.ReadFrom(resp.Body)
		return nil, fmt.Errorf("recover failed (status %d): %s", resp.StatusCode, strings.TrimSpace(errMsg.String()))
	}

	var rr RecoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, fmt.Errorf("decode recover response: %w", err)
	}
	if rr.TenantID == "" || rr.DeviceID == "" || rr.DeviceAPIKey == "" || rr.RepoPath == "" || rr.Password == "" {
		return nil, fmt.Errorf("recover response missing required fields")
	}

	return &EnrollmentResult{
		TenantID:     rr.TenantID,
		DeviceID:     rr.DeviceID,
		PrincipalID:  rr.PrincipalID,
		DeviceAPIKey: rr.DeviceAPIKey,
		RepoPath:     rr.RepoPath,
		Password:     rr.Password,
	}, nil
}

