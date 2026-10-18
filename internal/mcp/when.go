package mcp

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ParseWhen is the one rule for a `--since`-shaped argument, shared by the CLI
// and the agent tools so that an agent and an operator mean the same thing by the
// same words. It accepts either a duration ago ("24h", "30d", "2w") or an
// absolute timestamp, because both are what people reach for and neither is
// surprising. The CLI imports it from here, which it already does for the tools.
func ParseWhen(raw string) (time.Time, error) { return ParseWhenAt(raw, time.Now()) }

// ParseWhenAt is ParseWhen with the instant "ago" is counted from supplied, so
// the rule can be tested without a clock.
func ParseWhenAt(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if d, ok := ParseAgo(raw); ok {
		return now.Add(-d), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("not a duration like 24h or 30d nor a timestamp like 2026-01-30 or 2026-01-30T12:00:00Z")
}

// ParseAgo reads how long ago: a Go duration such as 90m or 24h, or whole days
// or weeks as 30d and 2w, because release comparisons are made in those and
// Go's own parser stops at hours. A sign is ignored: "-5d" and "5d" both mean
// five days ago. An amount a Duration cannot hold, about 292 years, is not a
// duration, as Go's own parser says of hours.
func ParseAgo(raw string) (time.Duration, bool) {
	if n := len(raw) - 1; n > 0 {
		var unit time.Duration
		switch raw[n] {
		case 'd':
			unit = 24 * time.Hour
		case 'w':
			unit = 7 * 24 * time.Hour
		}
		if unit != 0 {
			if count, err := strconv.Atoi(raw[:n]); err == nil {
				// The most negative integer has no positive twin, so negating it
				// leaves it negative. A product that wraps is negative too, and
				// "ago" reads a negative amount as a moment in the future, which
				// is how 400000000d came to mean the year 2318.
				if count < 0 {
					count = -count
				}
				if count < 0 || time.Duration(count) > math.MaxInt64/unit {
					return 0, false
				}
				return time.Duration(count) * unit, true
			}
		}
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, false
	}
	if d < 0 {
		d = -d
	}
	return d, true
}
