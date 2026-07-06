package controlapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"xentz-agent/internal/validation"
)

const maxErrorBodyBytes = 512

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("control API request failed (status %d)", e.StatusCode)
	}
	return fmt.Sprintf("control API request failed (status %d): %s", e.StatusCode, e.Body)
}

func (e *StatusError) AuthFailure() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
}

func New(baseURL, bearerToken string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if err := validation.ValidateServerURL(baseURL); err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(bearerToken),
		http:    &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) GetJSON(path string, out any) error {
	return c.DoJSON(http.MethodGet, path, nil, out)
}

func (c *Client) GetStatus(path string) (int, error) {
	return c.DoJSONStatus(http.MethodGet, path, nil, nil)
}

func (c *Client) PostJSON(path string, in, out any) error {
	return c.DoJSON(http.MethodPost, path, in, out)
}

func (c *Client) PutJSON(path string, in, out any) error {
	return c.DoJSON(http.MethodPut, path, in, out)
}

func (c *Client) DoJSON(method, path string, in, out any) error {
	_, err := c.DoJSONStatus(method, path, in, out)
	return err
}

func (c *Client) DoJSONStatus(method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return 0, fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if out != nil {
		req.Header.Set("Accept", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.StatusCode, &StatusError{
			StatusCode: resp.StatusCode,
			Body:       readErrorBody(resp.Body),
		}
	}

	if out == nil {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}
	return resp.StatusCode, nil
}

func readErrorBody(r io.Reader) string {
	var buf bytes.Buffer
	_, _ = io.CopyN(&buf, r, maxErrorBodyBytes)
	errStr := strings.TrimSpace(buf.String())
	errStr = strings.ReplaceAll(errStr, "\n", " ")
	errStr = strings.ReplaceAll(errStr, "\r", " ")
	if len(errStr) > 256 {
		errStr = errStr[:256] + "..."
	}
	return errStr
}
