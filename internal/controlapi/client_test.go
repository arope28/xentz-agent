package controlapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewValidatesBaseURLAndDefaults(t *testing.T) {
	if _, err := New("", "", time.Second); err == nil {
		t.Fatal("New with empty URL succeeded")
	}
	if _, err := New("file:///tmp/control", "", time.Second); err == nil {
		t.Fatal("New with invalid scheme succeeded")
	}

	client, err := New("https://control.example.com/", " token ", 0)
	if err != nil {
		t.Fatalf("New valid URL: %v", err)
	}
	if client.baseURL != "https://control.example.com" {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
	if client.token != "token" {
		t.Fatalf("token = %q", client.token)
	}
	if client.http.Timeout != 30*time.Second {
		t.Fatalf("timeout = %s, want 30s", client.http.Timeout)
	}
}

func TestDoJSONSendsAuthHeadersAndDecodesResponse(t *testing.T) {
	client := testClient("secret", func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/device/config" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q", got)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["device_id"] != "device-1" {
			t.Fatalf("device_id = %q", payload["device_id"])
		}

		return jsonResponse(http.StatusOK, `{"revision":3}`), nil
	})

	var out struct {
		Revision int `json:"revision"`
	}
	if err := client.PostJSON("/v1/device/config", map[string]string{"device_id": "device-1"}, &out); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if out.Revision != 3 {
		t.Fatalf("revision = %d, want 3", out.Revision)
	}
}

func TestDoJSONReturnsStatusError(t *testing.T) {
	client := testClient("", func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, "denied\ntry again"), nil
	})
	err := client.GetJSON("/v1/device/config", &struct{}{})
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %T, want *StatusError", err)
	}
	if statusErr.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", statusErr.StatusCode)
	}
	if !statusErr.AuthFailure() {
		t.Fatal("AuthFailure returned false for 403")
	}
	if strings.Contains(statusErr.Body, "\n") {
		t.Fatalf("body contains newline: %q", statusErr.Body)
	}
	if !strings.Contains(statusErr.Error(), "denied") {
		t.Fatalf("Error() = %q", statusErr.Error())
	}
}

func testClient(token string, roundTrip func(*http.Request) (*http.Response, error)) *Client {
	return &Client{
		baseURL: "https://control.example.com",
		token:   token,
		http:    &http.Client{Transport: roundTripper(roundTrip)},
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (rt roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
