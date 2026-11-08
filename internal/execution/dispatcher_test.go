package execution

import (
	"reflect"
	"testing"
	"time"
)

func TestCapacityBounds(t *testing.T) {
	valid := DefaultCapacity()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*Capacity){func(c *Capacity) { c.Workers = 0 }, func(c *Capacity) { c.Workers = 65 }, func(c *Capacity) { c.ActiveSteps = 0 }, func(c *Capacity) { c.ActiveSteps = 257 }, func(c *Capacity) { c.RunningExecutions = 0 }, func(c *Capacity) { c.RunningExecutions = 257 }, func(c *Capacity) { c.QueuedExecutions = 0 }, func(c *Capacity) { c.QueuedExecutions = 10001 }, func(c *Capacity) { c.QueueTimeout = 0 }, func(c *Capacity) { c.QueueTimeout = 25 * time.Hour }, func(c *Capacity) { c.ShutdownTimeout = 0 }, func(c *Capacity) { c.ShutdownTimeout = 6 * time.Minute }} {
		c := valid
		edit(&c)
		if c.Validate() == nil {
			t.Fatal(c)
		}
	}
}

func TestRecoverStoppedPreservesTerminalAndMarksUncertainty(t *testing.T) {
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	scenarios := []struct {
		name      string
		format    int
		state     State
		attempt   []Attempt
		deadline  *time.Time
		want      State
		uncertain bool
	}{
		{"legacy pending", 1, Pending, nil, nil, Failed, true},
		{"legacy running", 1, Running, nil, nil, Failed, true},
		{"format2 in flight", 2, Running, []Attempt{{Number: 1, State: Running, StartedAt: now}}, &later, Failed, true},
		{"format2 expired in flight", 2, Running, []Attempt{{Number: 1, State: Running, StartedAt: now}}, &now, Failed, true},
		{"format2 expired pending", 2, Pending, nil, &now, TimedOut, false},
		{"format2 expired before invocation", 2, Running, nil, &now, TimedOut, false},
		{"format2 safe pending", 2, Pending, nil, &later, Running, false},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			snap := Snapshot{ID: "execution", State: Running, Format: scenario.format, Deadline: scenario.deadline, Steps: []Step{{ID: "done", State: Succeeded, Output: []byte(`{"result":true}`)}, {ID: "work", State: scenario.state, Attempts: scenario.attempt}}}
			before := snap.Steps[0]
			got, events, err := RecoverStopped(snap, now)
			if err != nil || got.State != scenario.want || got.UncertainEffects != scenario.uncertain || !reflect.DeepEqual(got.Steps[0], before) {
				t.Fatal(got, events, err)
			}
			if scenario.want != Running && len(events) == 0 {
				t.Fatal("missing durable recovery event")
			}
			if len(scenario.attempt) > 0 && got.Steps[1].Attempts[0].Reason != "uncertain_effect" {
				t.Fatal(got)
			}
			if snap.Steps[1].State != scenario.state {
				t.Fatal("mutated original history")
			}
		})
	}
	for _, snap := range []Snapshot{{State: Succeeded}, {State: Running, Format: 99}} {
		if _, _, err := RecoverStopped(snap, now); err == nil {
			t.Fatal(snap)
		}
	}
}
