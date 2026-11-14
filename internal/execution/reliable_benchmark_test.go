package execution

import (
	"encoding/json"
	"testing"
	"time"
)

func BenchmarkReliablePolicy(b *testing.B) {
	for b.Loop() {
		snap := reliableFixture()
		now := snap.CreatedAt
		operations := []ReliableMutation{{Operation: StartStep, Step: "one"}, {Operation: StartAttempt, Step: "one"}, {Operation: FinishAttempt, Step: "one", Outcome: Failed, Reason: "action_failed"}, {Operation: StartAttempt, Step: "one"}, {Operation: FinishAttempt, Step: "one", Outcome: Succeeded, Output: json.RawMessage(`{}`)}, {Operation: FinishExecution}}
		for i, m := range operations {
			if i == 3 {
				now = now.Add(time.Second)
			}
			m.Version = snap.Version
			if _, err := ApplyReliable(&snap, m, now); err != nil {
				b.Fatal(err)
			}
		}
		if snap.State != Succeeded || snap.Steps[0].Attempt != 2 {
			b.Fatal(snap.State)
		}
	}
}
