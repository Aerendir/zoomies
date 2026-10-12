package assistant

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Only named fleet-status fields become prose. Missing figures stay missing,
// and repository names or arbitrary tool text cannot become demo advice.
type demoJobStats struct {
	Queued     *int   `json:"queued_jobs"`
	Running    *int   `json:"running_jobs"`
	Completed  *int   `json:"completed"`
	Passed     *int   `json:"succeeded"`
	Failed     *int   `json:"failed"`
	FleetFault *int   `json:"fleet_failed"`
	Cancelled  *int   `json:"cancelled"`
	Unknown    *int   `json:"unknown"`
	MedianWait *int64 `json:"median_wait_ms"`
	P95Wait    *int64 `json:"p95_wait_ms"`
}

func demoFleetSummary(raw string) string {
	var status struct {
		demoJobStats
		Window string        `json:"window"`
		Fleet  *demoJobStats `json:"fleet"`
		Hosts  *struct {
			Healthy *int `json:"healthy"`
			Total   *int `json:"total"`
		} `json:"hosts"`
		Runners *struct {
			Total *int `json:"total"`
			Busy  *int `json:"busy"`
			Idle  *int `json:"idle"`
		} `json:"runners"`
		Faults map[string]int `json:"faults"`
	}
	const unavailable = "The fleet lookup returned a response the demo model cannot summarise. Try again, or use a configured model provider for a fuller investigation."
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return unavailable
	}
	jobs := status.demoJobStats
	if status.Fleet != nil {
		// The top-level counts include jobs on other runners. When supplied,
		// this scope answers the question about this Zoomies fleet.
		jobs = *status.Fleet
	}
	var now, recent []string
	line := func(lines *[]string, label, text string) {
		if text != "" {
			*lines = append(*lines, "- **"+label+":** "+text+".")
		}
	}
	line(&now, "Jobs", demoCounts(demoCount{jobs.Queued, "waiting"}, demoCount{jobs.Running, "running"}))
	if status.Runners != nil {
		r := status.Runners
		line(&now, "Runners", demoCounts(demoCount{r.Total, "total"}, demoCount{r.Busy, "busy"}, demoCount{r.Idle, "idle"}))
	}
	if h := status.Hosts; h != nil && h.Healthy != nil && h.Total != nil && *h.Healthy >= 0 && *h.Total >= *h.Healthy {
		line(&now, "Hosts", fmt.Sprintf("%d of %d healthy", *h.Healthy, *h.Total))
	}
	line(&recent, "Completed jobs", demoCounts(demoCount{jobs.Completed, "finished"}, demoCount{jobs.Passed, "passed"}, demoCount{jobs.Failed, "failed"}, demoCount{jobs.Cancelled, "cancelled"}, demoCount{jobs.Unknown, "with an unknown outcome"}))
	if jobs.FleetFault != nil && *jobs.FleetFault >= 0 {
		line(&recent, "Runner-related job failures", fmt.Sprintf("%d", *jobs.FleetFault))
	}
	if jobs.MedianWait != nil && jobs.P95Wait != nil && *jobs.MedianWait >= 0 && *jobs.P95Wait >= 0 {
		line(&recent, "Queue wait", fmt.Sprintf("median %s; 95%% of measured waits were %s or less", demoSeconds(*jobs.MedianWait), demoSeconds(*jobs.P95Wait)))
	}
	if len(now)+len(recent) == 0 {
		return unavailable
	}
	sections := []string{"**Fleet summary**"}
	if len(now) > 0 {
		sections = append(sections, "**Right now**\n\n"+strings.Join(now, "\n"))
	}
	if len(recent) > 0 {
		label := "Recent outcomes"
		if window, err := time.ParseDuration(status.Window); err == nil && window > 0 {
			if window%time.Hour == 0 {
				label = "Over the last " + demoPeriod(int64(window/time.Hour), "hour")
			} else if window%time.Minute == 0 {
				label = "Over the last " + demoPeriod(int64(window/time.Minute), "minute")
			}
		}
		sections = append(sections, "**"+label+"**\n\n"+strings.Join(recent, "\n"))
	}
	next := "Open Jobs for individual results and Overview for current capacity."
	switch {
	case status.Faults["out_of_memory"] > 0:
		next = "A runner ran out of memory. Open the failed job in Jobs and review its memory limit and runner logs before changing its size."
	case jobs.FleetFault != nil && *jobs.FleetFault > 0:
		next = "Open the failed job in Jobs and review its runner fault and logs to understand the cause."
	case status.Hosts != nil && status.Hosts.Healthy != nil && status.Hosts.Total != nil && *status.Hosts.Healthy >= 0 && *status.Hosts.Healthy < *status.Hosts.Total:
		next = "Check the host health details in Hosts before relying on that capacity."
	case jobs.Queued != nil && *jobs.Queued > 0:
		next = "For waiting jobs, check Queue and Pools for matching labels and available capacity."
	}
	sections = append(sections, "**Next step**\n\n"+next)
	return strings.Join(sections, "\n\n")
}

func demoSeconds(ms int64) string {
	if ms > 0 && ms < 1000 {
		if ms == 1 {
			return "1 millisecond"
		}
		return fmt.Sprintf("%d milliseconds", ms)
	}
	number := strings.TrimSuffix(fmt.Sprintf("%.1f", float64(ms)/1000), ".0")
	if number == "1" {
		return "1 second"
	}
	return number + " seconds"
}

func demoPeriod(count int64, unit string) string {
	if count == 1 {
		return unit
	}
	return fmt.Sprintf("%d %ss", count, unit)
}

// Cancelled and unknown outcomes are useful when present, not as empty noise.
type demoCount struct {
	count *int
	label string
}

func demoCounts(values ...demoCount) string {
	parts := []string{}
	for _, v := range values {
		if v.count == nil || *v.count < 0 {
			continue
		}
		if *v.count == 0 && (v.label == "cancelled" || v.label == "with an unknown outcome") {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", *v.count, v.label))
	}
	return strings.Join(parts, "; ")
}
