package enroll

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"xentz-agent/internal/controlapi"
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
	client, err := controlapi.New(serverURL, "", 30*time.Second)
	if err != nil {
		return nil, err
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

	var rr RecoverResponse
	if err := client.PostJSON("/control/v1/recover", reqBody, &rr); err != nil {
		var statusErr *controlapi.StatusError
		if errors.As(err, &statusErr) {
			return nil, fmt.Errorf("recover failed (status %d): %s", statusErr.StatusCode, strings.TrimSpace(statusErr.Body))
		}
		return nil, fmt.Errorf("recover request failed: %w", err)
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
