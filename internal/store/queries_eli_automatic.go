package store

import (
	"context"
	"time"
)

// EliRepairUsage is how much of a repository's allowance the last 24 hours have
// spent, and when each of those attempts was made, oldest first, so a caller can
// say when the count will have fallen far enough to let another in.
type EliRepairUsage struct {
	Used  int
	Times []time.Time
}

// FreesAt is the instant the window will hold fewer than limit attempts, which
// is when the attempt that many from the oldest leaves it. With room to spare it
// is the oldest, the first thing to change. A limit lowered below what has been
// spent needs several to leave, and the oldest alone would promise a place that
// is not there.
func (u EliRepairUsage) FreesAt(limit int) time.Time {
	if len(u.Times) == 0 {
		return time.Time{}
	}
	i := min(max(u.Used-limit, 0), len(u.Times)-1)
	return u.Times[i].Add(24 * time.Hour)
}

// EliRepairUsage counts, for every repository with an attempt in the last 24
// hours, the attempts EnqueueEliRepair weighs against the daily limit: by anyone,
// by any trigger, and including a repair cleared from the history, because
// clearing hides a row and never gives its place back. It is the same window and
// the same rows on purpose; an allowance shown to an administrator that differed
// from the one enforced would say a repository has room when the next failure is
// refused.
func (s *Store) EliRepairUsage(ctx context.Context) (map[string]EliRepairUsage, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT repo, created_at FROM eli_repairs WHERE created_at>=? ORDER BY repo, created_at`, ms(s.Now().Add(-24*time.Hour)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]EliRepairUsage{}
	for rows.Next() {
		var repo string
		var created int64
		if err := rows.Scan(&repo, &created); err != nil {
			return nil, err
		}
		u := out[repo]
		u.Used++
		u.Times = append(u.Times, at(created))
		out[repo] = u
	}
	return out, rows.Err()
}

// EliAutomaticLast is the newest automatic repair a repository still shows in
// its history, in the few fields a row of the overview needs.
type EliAutomaticLast struct {
	ID         string    `json:"id"`
	State      string    `json:"state"`
	PullNumber int       `json:"pull_number"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// EliAutomaticActivity is what the automation is doing at one repository now and
// what it last did.
type EliAutomaticActivity struct {
	Running int
	Last    *EliAutomaticLast
}

// EliAutomaticActivity answers for the repositories it is asked about, and only
// those, so the scan for the newest row of each can stop as soon as every one has
// been found. A repository the automation has never touched has no entry, and a
// repair the administrator cleared from the history is not offered back as the
// latest: clearing is how they say they are finished looking at it.
func (s *Store) EliAutomaticActivity(ctx context.Context, repos []string) (map[string]EliAutomaticActivity, error) {
	out := map[string]EliAutomaticActivity{}
	want := make(map[string]bool, len(repos))
	for _, repo := range repos {
		want[repo] = true
	}
	if len(want) == 0 {
		return out, nil
	}

	running, err := s.read.QueryContext(ctx, `SELECT repo, count(*) FROM eli_repairs WHERE trigger='automatic' AND state IN `+eliRepairActive+` GROUP BY repo`)
	if err != nil {
		return nil, err
	}
	defer running.Close()
	for running.Next() {
		var repo string
		var n int
		if err := running.Scan(&repo, &n); err != nil {
			return nil, err
		}
		if want[repo] {
			out[repo] = EliAutomaticActivity{Running: n}
		}
	}
	if err := running.Err(); err != nil {
		return nil, err
	}

	newest, err := s.read.QueryContext(ctx, `SELECT repo, id, state, pull_number, updated_at FROM eli_repairs WHERE trigger='automatic' AND cleared_at IS NULL ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer newest.Close()
	found := map[string]bool{}
	for newest.Next() && len(found) < len(want) {
		var repo string
		var last EliAutomaticLast
		var updated int64
		if err := newest.Scan(&repo, &last.ID, &last.State, &last.PullNumber, &updated); err != nil {
			return nil, err
		}
		if !want[repo] || found[repo] {
			continue
		}
		found[repo] = true
		last.UpdatedAt = at(updated)
		activity := out[repo]
		activity.Last = &last
		out[repo] = activity
	}
	return out, newest.Err()
}
