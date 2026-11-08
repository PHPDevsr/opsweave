package execution

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestTransitionMatrix(t *testing.T) {
	states := []State{Pending, Queued, Running, Succeeded, Failed, Cancelled, TimedOut, Skipped, "unknown"}
	for _, step := range []bool{false, true} {
		allowed := map[[2]State]bool{}
		edges := [][2]State{{Running, Succeeded}, {Running, Failed}, {Running, Cancelled}, {Running, TimedOut}, {Pending, Cancelled}}
		if step {
			edges = append(edges, [2]State{Pending, Running}, [2]State{Pending, Skipped})
		} else {
			edges = append(edges, [2]State{Pending, Queued}, [2]State{Queued, Running}, [2]State{Queued, Cancelled})
		}
		for _, edge := range edges {
			allowed[edge] = true
		}
		for _, from := range states {
			for _, to := range states {
				if CanTransition(from, to, step) != allowed[[2]State{from, to}] {
					t.Fatalf("%s -> %s step=%v", from, to, step)
				}
			}
		}
	}
	for _, s := range states {
		want := s == Succeeded || s == Failed || s == Cancelled || s == TimedOut || s == Skipped
		if Terminal(s) != want {
			t.Fatal(s)
		}
	}
}

func TestAggregateTransitions(t *testing.T) {
	fresh := func() Snapshot {
		return Snapshot{State: Running, Steps: []Step{{ID: "one", State: Pending}, {ID: "two", State: Pending}}}
	}
	cases := []struct {
		name  string
		edit  func(*Snapshot)
		m     Mutation
		valid bool
	}{
		{"start", nil, Mutation{Step: "one", To: Running}, true},
		{"dependency", nil, Mutation{Step: "two", To: Running}, false},
		{"unknown", nil, Mutation{Step: "absent", To: Running}, false},
		{"stale", nil, Mutation{Version: 1, Step: "one", To: Running}, false},
		{"terminal", func(s *Snapshot) { s.State = Succeeded }, Mutation{To: Failed}, false},
		{"incomplete", nil, Mutation{To: Succeeded}, false},
		{"invalid execution edge", nil, Mutation{To: Queued}, false},
		{"extra reason", nil, Mutation{To: Failed, Reason: "secret"}, false},
		{"extra output", nil, Mutation{To: Failed, Output: json.RawMessage(`{}`)}, false},
		{"failed without failure", func(s *Snapshot) {
			for i := range s.Steps {
				s.Steps[i].State = Succeeded
			}
		}, Mutation{To: Failed}, false},
		{"success", func(s *Snapshot) {
			for i := range s.Steps {
				s.Steps[i].State = Succeeded
			}
		}, Mutation{To: Succeeded}, true},
		{"failure", func(s *Snapshot) { s.Steps[0].State = Failed; s.Steps[1].State = Skipped }, Mutation{To: Failed}, true},
		{"no false success", func(s *Snapshot) { s.Steps[0].State = Failed; s.Steps[1].State = Skipped }, Mutation{To: Succeeded}, false},
		{"skip", func(s *Snapshot) { s.Steps[0].State = Failed }, Mutation{Step: "two", To: Skipped, Reason: "dependency_failed"}, true},
		{"invalid skip", nil, Mutation{Step: "two", To: Skipped, Reason: "dependency_failed"}, false},
		{"no root skip", nil, Mutation{Step: "one", To: Skipped, Reason: "dependency_failed"}, false},
		{"unsafe reason", nil, Mutation{Step: "one", To: Running, Reason: "secret"}, false},
		{"cancel pending", nil, Mutation{Step: "one", To: Cancelled, Reason: "cancelled"}, true},
		{"finish", func(s *Snapshot) { s.Steps[0].State = Running }, Mutation{Step: "one", To: Succeeded, Output: json.RawMessage(`{"ok":true}`)}, true},
		{"bad JSON", func(s *Snapshot) { s.Steps[0].State = Running }, Mutation{Step: "one", To: Succeeded, Output: json.RawMessage(`[]`)}, false},
		{"running output", nil, Mutation{Step: "one", To: Running, Output: json.RawMessage(`{}`)}, false},
		{"too large", func(s *Snapshot) { s.Steps[0].State = Running }, Mutation{Step: "one", To: Succeeded, Output: make([]byte, 65537)}, false},
		{"total limit", func(s *Snapshot) { s.Steps[0].State = Running; s.Steps[1].Output = make([]byte, 1<<20) }, Mutation{Step: "one", To: Succeeded, Output: json.RawMessage(`{}`)}, false},
		{"one runner", func(s *Snapshot) { s.Steps[1].State = Running }, Mutation{Step: "one", To: Running}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fresh()
			if tc.edit != nil {
				tc.edit(&s)
			}
			err := Advance(&s, tc.m, time.Now().UTC())
			if (err == nil) != tc.valid {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestIdentifiersAndSafeFailure(t *testing.T) {
	for _, id := range []string{"", "bad", "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ", "0123456789abcdef0123456789abcdef"} {
		if ValidID(id) != (id == "0123456789abcdef0123456789abcdef") {
			t.Fatal(id)
		}
	}
	cause := errors.New("private database path")
	err := &Failure{Cause: cause}
	if !errors.Is(err, cause) || err.Error() == cause.Error() {
		t.Fatal(err)
	}
}

func BenchmarkTransition(b *testing.B) {
	for b.Loop() {
		s := Snapshot{State: Running, Steps: []Step{{ID: "one", State: Pending}}}
		if err := Advance(&s, Mutation{Step: "one", To: Running}, time.Unix(0, 0)); err != nil {
			b.Fatal(err)
		}
	}
}
