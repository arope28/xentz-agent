package enroll

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"
)

// DeviceMetadata contains device information sent during enrollment
type DeviceMetadata struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

// EnrollmentRequest is sent to the server during enrollment
// Note: Token is sent in Authorization header, not in body
type EnrollmentRequest struct {
	UserID   string         `json:"user_id,omitempty"` // User identifier for repository path construction
	Metadata DeviceMetadata `json:"metadata"`
}

// EnrollmentResponse is received from the server
type EnrollmentResponse struct {
	TenantID     string `json:"tenant_id"`
	DeviceID     string `json:"device_id"`
	DeviceAPIKey string `json:"device_api_key"` // Long-lived, revocable API key for future requests
	RepoPath     string `json:"repo_path"`      // Full repository URL or path
	Password     string `json:"password,omitempty"` // Optional: server-generated password
}

// EnrollmentResult contains the enrollment data to store in config
type EnrollmentResult struct {
	TenantID     string
	DeviceID     string
	DeviceAPIKey string // Long-lived API key for fetching config
	RepoPath     string
	Password     string
}

// GetDeviceMetadata collects device metadata for enrollment
func GetDeviceMetadata() (DeviceMetadata, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return DeviceMetadata{}, fmt.Errorf("get hostname: %w", err)
	}

	return DeviceMetadata{
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	}, nil
}

// GetUserID returns the user identifier (username by default)
func GetUserID() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}
	return currentUser.Username, nil
}

// Enroll calls the control plane API to enroll the device and get server-issued identifiers
func Enroll(token, serverURL string) (*EnrollmentResult, error) {
	if token == "" {
		return nil, fmt.Errorf("install token is required")
	}
	if serverURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}

	// Collect device metadata
	metadata, err := GetDeviceMetadata()
	if err != nil {
		return nil, fmt.Errorf("collect device metadata: %w", err)
	}

	// Get user ID
	userID, err := GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user ID: %w", err)
	}

	// Prepare enrollment request (token goes in Authorization header, not body)
	reqBody := EnrollmentRequest{
		UserID:   userID,
		Metadata: metadata,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal enrollment request: %w", err)
	}

	// Make POST request to /v1/install with Authorization Bearer header
	url := fmt.Sprintf("%s/v1/install", serverURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Set timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enrollment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errMsg bytes.Buffer
		errMsg.ReadFrom(resp.Body)
		return nil, fmt.Errorf("enrollment failed (status %d): %s", resp.StatusCode, errMsg.String())
	}

	// Parse response
	var enrollmentResp EnrollmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&enrollmentResp); err != nil {
		return nil, fmt.Errorf("decode enrollment response: %w", err)
	}

	// Validate response
	if enrollmentResp.TenantID == "" {
		return nil, fmt.Errorf("server did not return tenant_id")
	}
	if enrollmentResp.DeviceID == "" {
		return nil, fmt.Errorf("server did not return device_id")
	}
	if enrollmentResp.DeviceAPIKey == "" {
		return nil, fmt.Errorf("server did not return device_api_key")
	}
	if enrollmentResp.RepoPath == "" {
		return nil, fmt.Errorf("server did not return repo_path")
	}

	// Construct full repository path if needed
	repoPath := enrollmentResp.RepoPath
	// If server returns a base path, append user_id
	// If server returns full path, use it as-is
	// We assume server returns full path including user_id, but if not, we can construct it
	// For now, use the repo_path as-is since server should return complete path

	return &EnrollmentResult{
		TenantID:     enrollmentResp.TenantID,
		DeviceID:     enrollmentResp.DeviceID,
		DeviceAPIKey: enrollmentResp.DeviceAPIKey,
		RepoPath:     repoPath,
		Password:     enrollmentResp.Password,
	}, nil
}

// IsEnrolled checks if the device is already enrolled (has DeviceID)
func IsEnrolled(tenantID, deviceID string) bool {
	return tenantID != "" && deviceID != ""
}

// GetOrCreateUserID gets the user ID, creating and storing it if needed
func GetOrCreateUserID(configDir string) (string, error) {
	userIDFile := filepath.Join(configDir, "user_id")

	// Try to read existing user ID
	if data, err := os.ReadFile(userIDFile); err == nil {
		userID := string(data)
		if userID != "" {
			return userID, nil
		}
	}

	// Get system username
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("get current user: %w", err)
	}
	userID := currentUser.Username

	// Store it for future use
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(userIDFile, []byte(userID), 0o600); err != nil {
		return "", fmt.Errorf("write user ID: %w", err)
	}

	return userID, nil
}

