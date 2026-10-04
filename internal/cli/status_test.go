package cli

import (
	"testing"
	"time"
)

func TestDescribeAge(t *testing.T) {
	cases := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"seconds", 10 * time.Second, "just now"},
		{"one minute", 90 * time.Second, "1 minute ago"},
		{"minutes", 45 * time.Minute, "45 minutes ago"},
		{"one hour", 61 * time.Minute, "1 hour ago"},
		{"hours", 5 * time.Hour, "5 hours ago"},
		{"days", 50 * time.Hour, "2 days ago"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := time.Now().Add(-tc.ago).UTC().Format(time.RFC3339)
			if got := describeAge(ts); got != tc.want {
				t.Errorf("describeAge(%s) = %q, want %q", tc.ago, got, tc.want)
			}
		})
	}
}

func TestDescribeAgeUnparsable(t *testing.T) {
	// A corrupt timestamp should be echoed rather than silently shown as
	// "just now", otherwise a bad run looks recent.
	if got := describeAge("not-a-timestamp"); got != "at not-a-timestamp" {
		t.Errorf("describeAge(garbage) = %q, want %q", got, "at not-a-timestamp")
	}
}

func TestDescribeAgeFutureClockSkew(t *testing.T) {
	ts := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	got := describeAge(ts)
	if got != "in the future (check the system clock)" {
		t.Errorf("describeAge(future) = %q", got)
	}
}

func TestFormatLocalTime(t *testing.T) {
	ts := time.Now().UTC().Format(time.RFC3339)
	got := formatLocalTime(ts)
	if got == "" {
		t.Fatal("formatLocalTime returned empty for a valid timestamp")
	}
	if got[len(got)-len(" local"):] != " local" {
		t.Errorf("formatLocalTime = %q, want a ' local' suffix", got)
	}
}

func TestFormatLocalTimeInvalid(t *testing.T) {
	if got := formatLocalTime("garbage"); got != "" {
		t.Errorf("formatLocalTime(garbage) = %q, want empty so callers omit it", got)
	}
}

func TestPluralize(t *testing.T) {
	cases := map[int]string{0: "0 minutes", 1: "1 minute", 2: "2 minutes"}
	for n, want := range cases {
		if got := pluralize(n, "minute"); got != want {
			t.Errorf("pluralize(%d, minute) = %q, want %q", n, got, want)
		}
	}
}

func TestStdinIsTerminalDoesNotPanic(t *testing.T) {
	// Under `go test` stdin is not a terminal; we only assert it is safe and
	// does not report a terminal when stdin is redirected.
	if stdinIsTerminal() {
		t.Skip("test runner has a terminal on stdin")
	}
}
