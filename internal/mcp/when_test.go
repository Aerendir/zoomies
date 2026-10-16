package mcp

import (
	"testing"
	"time"

	"github.com/eyupio/zoomies/internal/analysis"
)

// `--since 30d` is typed by operators and by agents, in the CLI and in the
// tools, and both used to carry their own copy of the parser with a comment
// promising they meant the same thing. They are one function now, and this is
// what it means.
func TestParseAgoReadsDurationsDaysAndWeeksAndIgnoresASign(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want time.Duration
	}{
		{"90m", 90 * time.Minute},
		{"24h", 24 * time.Hour},
		{"1.5h", 90 * time.Minute},
		{"30d", 30 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"0d", 0},
		// A sign does not matter: -5d and 5d both mean five days ago.
		{"-5d", 5 * 24 * time.Hour},
		{"-90m", 90 * time.Minute},
	} {
		got, ok := ParseAgo(tc.in)
		if !ok || got != tc.want {
			t.Errorf("ParseAgo(%q) = %v, %v; want %v", tc.in, got, ok, tc.want)
		}
	}
	for _, bad := range []string{"", "d", "w", "abc", "2y", "12", "1.5d", "5 d"} {
		if got, ok := ParseAgo(bad); ok {
			t.Errorf("ParseAgo(%q) = %v, true; it is not a duration", bad, got)
		}
	}
}

func TestParseWhenCountsAgoFromTheInstantItIsGivenAndReadsTimestamps(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in   string
		want time.Time
	}{
		{"24h", time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)},
		{" 2w ", time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)},
		{"2026-01-30T12:00:00Z", time.Date(2026, 1, 30, 12, 0, 0, 0, time.UTC)},
		{"2026-01-30T12:00:00+02:00", time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC)},
		{"2026-01-30T12:00:00", time.Date(2026, 1, 30, 12, 0, 0, 0, time.UTC)},
		{"2026-01-30", time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)},
	} {
		got, err := ParseWhenAt(tc.in, now)
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("ParseWhenAt(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"", "yesterday", "30 days", "2026-13-40", "12/01/2026"} {
		if _, err := ParseWhenAt(bad, now); err == nil {
			t.Errorf("ParseWhenAt(%q) was accepted", bad)
		}
	}
	if _, err := ParseWhenAt("nonsense", now); err == nil || err.Error() == "" {
		t.Error("a refused value should say what would have been accepted")
	}
}

func TestChangeFigureNamesTheUnitAndAdmitsWhenThereIsNone(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		v    *float64
		unit string
		want string
	}{
		{nil, "ms", "unknown"},
		{nil, "", "unknown"},
		{f(1500), "ms", analysis.Words(1500 * time.Millisecond)},
		{f(0.1234), "rate", "12.3%"},
		{f(2.5), "", "2.5"},
		{f(3), "count", "3"},
	} {
		if got := ChangeFigure(tc.v, tc.unit); got != tc.want {
			t.Errorf("ChangeFigure(%v, %q) = %q, want %q", tc.v, tc.unit, got, tc.want)
		}
	}
}
