package localui

import (
	"strings"
	"testing"

	"xentz-agent/internal/config"
)

func TestDescribeSchedule(t *testing.T) {
	if got := describeSchedule("02:30"); got != "Daily at 02:30" {
		t.Errorf("describeSchedule(02:30) = %q", got)
	}
	// Never invent a schedule the user did not configure.
	for _, in := range []string{"", "   "} {
		got := describeSchedule(in)
		if !strings.Contains(strings.ToLower(got), "not configured") {
			t.Errorf("describeSchedule(%q) = %q, want it to say not configured", in, got)
		}
	}
}

func TestDescribeRetention(t *testing.T) {
	cases := []struct {
		name string
		in   config.Retention
		want []string
	}{
		{"daily and weekly", config.Retention{KeepDaily: 7, KeepWeekly: 4}, []string{"keep_daily=7", "keep_weekly=4"}},
		{"with prune", config.Retention{KeepLast: 5, Prune: true}, []string{"keep_last=5", "prune"}},
		{"prune only", config.Retention{Prune: true}, []string{"prune"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := describeRetention(tc.in)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("describeRetention(%+v) = %q, missing %q", tc.in, got, want)
				}
			}
		})
	}

	if got := describeRetention(config.Retention{}); got != "Not configured" {
		t.Errorf("describeRetention(empty) = %q, want %q", got, "Not configured")
	}
}
