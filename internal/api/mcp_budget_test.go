package api

import (
	"fmt"
	"maps"
	"testing"
)

// budgetStep is one move the budget is asked to make: a take, which must be
// allowed or refused, or a give, which returns a slot.
type budgetStep struct {
	// take spends a slot for key; when it is false the step gives one back.
	take bool
	key  string
	// wantOK is what a take must answer.
	wantOK bool
}

// The budget is a keyed semaphore, not a rate limiter: what it bounds is how
// many answers one caller may be holding at once, so a caller that asks for
// a third is refused while a caller that has given one back is let in.
func TestTheInProcessBudgetRefusesTheThirdCallOfOneCaller(t *testing.T) {
	// Sixteen callers, then a seventeenth, which the fleet-wide ceiling
	// refuses even though each of them holds only one answer.
	fleet := make([]budgetStep, 0, inProcessTotal+1)
	for i := 0; i < inProcessTotal; i++ {
		fleet = append(fleet, budgetStep{take: true, key: fmt.Sprintf("caller %d", i), wantOK: true})
	}
	fleet = append(fleet, budgetStep{take: true, key: fmt.Sprintf("caller %d", inProcessTotal), wantOK: false})

	cases := []struct {
		name  string
		steps []budgetStep
	}{
		{
			name: "a caller may hold two answers and no more",
			steps: []budgetStep{
				{take: true, key: "token agent (viewer)", wantOK: true},
				{take: true, key: "token agent (viewer)", wantOK: true},
				{take: true, key: "token agent (viewer)", wantOK: false},
			},
		},
		{
			name:  "the fleet ceiling refuses a seventeenth caller",
			steps: fleet,
		},
		{
			name: "giving a slot back makes room for the same caller",
			steps: []budgetStep{
				{take: true, key: "token agent (viewer)", wantOK: true},
				{take: true, key: "token agent (viewer)", wantOK: true},
				{take: false, key: "token agent (viewer)"},
				{take: true, key: "token agent (viewer)", wantOK: true},
				{take: true, key: "token agent (viewer)", wantOK: false},
			},
		},
		{
			name: "a call without an identity is bounded like any other caller",
			steps: []budgetStep{
				{take: true, key: "", wantOK: true},
				{take: true, key: "", wantOK: true},
				{take: true, key: "", wantOK: false},
			},
		},
		{
			name: "a give for a caller holding nothing changes nothing",
			steps: []budgetStep{
				{take: false, key: "token nobody (viewer)"},
				{take: true, key: "token agent (viewer)", wantOK: true},
				{take: true, key: "token agent (viewer)", wantOK: true},
				{take: true, key: "token agent (viewer)", wantOK: false},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &inProcessBudget{}
			outstanding := map[string]int{}
			for i, s := range c.steps {
				if s.take {
					ok, retry := b.take(s.key)
					if ok != s.wantOK {
						t.Fatalf("step %d: taking for %q was allowed %v, wanted %v", i, s.key, ok, s.wantOK)
					}
					if !ok && retry <= 0 {
						t.Errorf("step %d: a refusal must say how long to wait", i)
					}
					if ok {
						outstanding[s.key]++
					}
				} else {
					b.give(s.key)
					if outstanding[s.key] > 0 {
						outstanding[s.key]--
						if outstanding[s.key] == 0 {
							delete(outstanding, s.key)
						}
					}
				}
				if !maps.Equal(b.held, outstanding) {
					t.Fatalf("step %d: the budget holds %v, wanted %v", i, b.held, outstanding)
				}
				slots := 0
				for _, n := range outstanding {
					slots += n
				}
				if b.totalHeld != slots {
					t.Fatalf("step %d: the budget holds %d answers in all, wanted %d", i, b.totalHeld, slots)
				}
			}
		})
	}
}
