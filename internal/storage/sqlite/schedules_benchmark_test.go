package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/scheduler"
)

func BenchmarkScheduleDueScan(b *testing.B) {
	s, _ := benchmarkStore(b)
	b.Cleanup(func() { closeBenchmarkStore(b, s) })
	candidate := benchmarkCandidates(b, 1)[0]
	source := strings.Replace(string(candidate.Source), "spec:", "spec:\n  triggers: [{id: daily, type: cron, schedule: '0 0 31 2 *', timezone: UTC}]", 1)
	catalog := registry.Catalog{"test.echo": {ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"message": {Type: "string", Required: true}}}}}
	candidates, err := registry.PrepareDispatcher(b.Context(), []registry.Source{{Data: []byte(source)}}, catalog)
	if err != nil {
		b.Fatal(err)
	}
	rows, err := s.Apply(b.Context(), candidates)
	if err != nil {
		b.Fatal(err)
	}
	f, err := s.BeginDispatcher(b.Context(), testOwner, false)
	if err != nil {
		b.Fatal(err)
	}
	service := scheduler.Service{Repository: s, Preflight: func(context.Context, registry.Record) error { b.Fatal("unexpected admission"); return nil }}
	cap := execution.DefaultCapacity()
	now := rows[0].CreatedAt.Truncate(time.Minute).Add(time.Minute)
	b.ReportAllocs()
	for b.Loop() {
		report, err := service.Tick(b.Context(), f, cap, now)
		if err != nil || report.Scanned != 1 || report.Admitted != 0 {
			b.Fatal(report, err)
		}
		now = now.Add(time.Minute)
	}
}
