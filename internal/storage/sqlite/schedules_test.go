package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PHPDevsr/opsweave/internal/action"
	"github.com/PHPDevsr/opsweave/internal/engine"
	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/schedule"
	"github.com/PHPDevsr/opsweave/internal/scheduler"
)

func scheduledFixture(t *testing.T) (*Store, registry.Record, execution.Fence, time.Time, string) {
	t.Helper()
	s, r, path := reliableStoreFixture(t)
	source := strings.Replace(string(r.Source), "spec:", "spec:\n  triggers: [{id: every, type: cron, schedule: '* * * * *', timezone: UTC, missed_run: catch_up_once}]", 1)
	r = dispatcherRevision(t, s, []byte(source))
	f, err := s.BeginDispatcher(t.Context(), testOwner, false)
	if err != nil {
		t.Fatal(err)
	}
	now := r.CreatedAt.Truncate(time.Minute).Add(time.Minute + 10*time.Second)
	return s, r, f, now, path
}

type interruptibleScheduleScan struct {
	scheduler.Repository
	calls   atomic.Int32
	entered chan struct{}
}

func (r *interruptibleScheduleScan) DueSchedules(ctx context.Context, f execution.Fence, now time.Time) ([]scheduler.Entry, error) {
	if r.calls.Add(1) == 1 {
		return r.Repository.DueSchedules(ctx, f, now)
	}
	close(r.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestSchedulerDispatcherCompletesAndInterrupts(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		name := "completion"
		if interrupt {
			name = "interrupt during scan"
		}
		t.Run(name, func(t *testing.T) {
			s, r, f, _, _ := scheduledFixture(t)
			now := time.Now().UTC()
			cursor := now.Truncate(time.Minute).Add(-time.Minute)
			if _, err := s.db.ExecContext(t.Context(), "UPDATE schedules SET cursor_at=?,checked_minute=?,retry_at=0 WHERE workflow_name=?", cursor.Format(time.RFC3339Nano), cursor.Unix(), r.Name); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			runtime, err := action.New([]action.Registration{{Descriptor: registry.Descriptor{ID: "test.echo", Version: "1", APIVersion: "opsweave.io/v1", Inputs: registry.Field{Type: "object", Properties: map[string]registry.Field{"message": {Type: "string", Required: true}}}}, Run: func(ctx context.Context, _ map[string]any) (map[string]any, error) {
				calls.Add(1)
				if interrupt {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return map[string]any{}, nil
			}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			e := engine.Engine{Workflows: s, Executions: s, Actions: runtime}
			repo := &interruptibleScheduleScan{Repository: s, entered: make(chan struct{})}
			d := engine.Dispatcher{Engine: e, Repository: s, Fence: f, Capacity: execution.DefaultCapacity(), Scheduler: &scheduler.Service{Repository: repo, Preflight: e.ValidateScheduled}, Now: func() time.Time {
				if interrupt && calls.Load() > 0 {
					return now.Add(2 * time.Second)
				}
				return now
			}}
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			done := make(chan struct{})
			var runErr error
			go func() { defer close(done); _, runErr = d.Run(ctx, "") }()
			t.Cleanup(func() { cancel(); <-done })
			want := execution.Succeeded
			if interrupt {
				select {
				case <-repo.entered:
				case <-done:
					t.Fatal(runErr)
				case <-ctx.Done():
					t.Fatal("scheduler scan never reached")
				}
				want = execution.Cancelled
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Fatal("interrupt did not join dispatcher")
				}
			} else {
				ticker := time.NewTicker(20 * time.Millisecond)
				defer ticker.Stop()
				for {
					rows, err := s.Executions(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if len(rows) == 1 && execution.Terminal(rows[0].State) {
						break
					}
					select {
					case <-ticker.C:
					case <-done:
						t.Fatal(runErr)
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
				cancel()
				<-done
			}
			if !errors.Is(runErr, context.Canceled) {
				t.Fatal(runErr)
			}
			rows, err := s.Executions(t.Context())
			if err != nil || len(rows) != 1 || rows[0].State != want || calls.Load() != 1 {
				t.Fatal(rows, calls.Load(), err)
			}
			job, err := s.Execution(t.Context(), rows[0].ID)
			if err != nil || job.Trigger == nil || !job.Trigger.ScheduledAt.Equal(now.Truncate(time.Minute)) {
				t.Fatal(job, err)
			}
		})
	}
}

func scheduleService(s *Store) *scheduler.Service {
	return &scheduler.Service{Repository: s, Preflight: func(context.Context, registry.Record) error { return nil }}
}

func TestScheduledAdmissionIdentityRestartAndHistory(t *testing.T) {
	s, r, f, now, path := scheduledFixture(t)
	service := scheduleService(s)
	cap := execution.DefaultCapacity()
	report, err := service.Tick(t.Context(), f, cap, r.CreatedAt)
	if err != nil || report.Admitted != 0 {
		t.Fatal("backfilled before deployment", report, err)
	}
	report, err = service.Tick(t.Context(), f, cap, now)
	if err != nil || report.Admitted != 1 {
		t.Fatal(report, err)
	}
	rows, err := s.Executions(t.Context())
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	snap, err := s.Execution(t.Context(), rows[0].ID)
	if err != nil || snap.Trigger == nil || snap.Trigger.ID != "every" || !snap.Trigger.ScheduledAt.Equal(now.Truncate(time.Minute)) || snap.Revision != r.Revision || snap.State != execution.Queued {
		t.Fatal(snap, err)
	}
	report, err = service.Tick(t.Context(), f, cap, now.Add(time.Second))
	if err != nil || report.Admitted != 0 {
		t.Fatal(report, err)
	}
	// Replay an old cursor as a fault simulation: durable occurrence identity still wins.
	if _, err = s.db.ExecContext(t.Context(), "UPDATE schedules SET cursor_at=?,checked_minute=?,retry_at=0,version=version+1", now.Add(-time.Minute).Format(time.RFC3339Nano), now.Add(-time.Minute).Truncate(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	other := openTest(t, path)
	fresh, err := other.BeginDispatcher(t.Context(), strings.Repeat("b", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	report, err = scheduleService(other).Tick(t.Context(), fresh, cap, now)
	if err != nil || report.Admitted != 0 {
		t.Fatal("duplicate admitted after restart", report, err)
	}
	rows, err = s.Executions(t.Context())
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	events, err := s.ScheduleEvents(t.Context(), r.Name, 1)
	if err != nil || len(events.Events) != 1 || events.Events[0].Code != "admitted" || events.Events[0].Execution != snap.ID {
		t.Fatal(events, err)
	}
	for _, table := range []string{"schedule_events", "scheduled_executions"} {
		if _, err = s.db.ExecContext(t.Context(), "DELETE FROM "+table); err == nil {
			t.Fatal("mutable automatic history", table)
		}
	}
}

func TestScheduleQueueFullRetainsExactPendingIntent(t *testing.T) {
	s, r, f, now, _ := scheduledFixture(t)
	cap := execution.DefaultCapacity()
	cap.QueuedExecutions = 1
	manual, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	service := scheduleService(s)
	report, err := service.Tick(t.Context(), f, cap, now)
	if err != nil || report.Deferred != 1 || report.Admitted != 0 {
		t.Fatal(report, err)
	}
	entries, err := s.Schedules(t.Context(), r.Name)
	if err != nil || entries[0].Pending == nil || !entries[0].Pending.Equal(now.Truncate(time.Minute)) || entries[0].LastFailure != "queue_full" {
		t.Fatal(entries, err)
	}
	report, err = service.Tick(t.Context(), f, cap, now.Add(time.Minute))
	if err != nil || report.Deferred != 1 {
		t.Fatal(report, err)
	}
	events, err := s.ScheduleEvents(t.Context(), r.Name, 10)
	if err != nil || len(events.Events) != 1 || events.Events[0].Code != "queue_full" {
		t.Fatal("repeated failure spam", events, err)
	}
	snap, err := s.ClaimExecution(t.Context(), f, manual.Snapshot.ID, r, cap)
	if err != nil {
		t.Fatal(err)
	}
	snap, err = s.RequestReliableCancellation(t.Context(), snap.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []execution.Operation{execution.CancelIdleSteps, execution.FinishExecution} {
		snap, err = s.ChangeDispatch(t.Context(), f, execution.ReliableMutation{ID: snap.ID, Owner: f.Owner, Version: snap.Version, Operation: op})
		if err != nil {
			t.Fatal(err)
		}
	}
	report, err = service.Tick(t.Context(), f, cap, now.Add(2*time.Minute))
	if err != nil || report.Admitted != 1 {
		t.Fatal(report, err)
	}
	events, err = s.ScheduleEvents(t.Context(), r.Name, 10)
	if err != nil || events.Events[0].ScheduledAt == nil || !events.Events[0].ScheduledAt.Equal(now.Truncate(time.Minute)) {
		t.Fatal("changed pending identity", events, err)
	}
}

func TestScheduleLifecycleFencesAutomaticButAllowsManualClaims(t *testing.T) {
	s, r, f, now, _ := scheduledFixture(t)
	cap := execution.DefaultCapacity()
	if _, err := scheduleService(s).Tick(t.Context(), f, cap, now); err != nil {
		t.Fatal(err)
	}
	automatic, err := s.Executions(t.Context())
	if err != nil || len(automatic) != 1 {
		t.Fatal(automatic, err)
	}
	if _, err = s.SetEnabled(t.Context(), r.Name, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetEnabled(t.Context(), r.Name, true); err != nil {
		t.Fatal(err)
	}
	// Even a re-enable cannot revive a queued admission from an invalidated lifecycle epoch.
	cancelled, err := s.ClaimExecution(t.Context(), f, automatic[0].ID, r, cap)
	if err != nil || cancelled.State != execution.Cancelled || cancelled.Steps[0].Reason != "workflow_disabled" {
		t.Fatal(cancelled, err)
	}
	if _, err = s.SetEnabled(t.Context(), r.Name, false); err != nil {
		t.Fatal(err)
	}
	report, err := scheduleService(s).Tick(t.Context(), f, cap, now.Add(time.Hour))
	if err != nil || report.Admitted != 0 {
		t.Fatal(report, err)
	}
	manual, err := s.Enqueue(t.Context(), r, execution.QueueAdmission{Capacity: cap})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.ClaimExecution(t.Context(), f, manual.Snapshot.ID, r, cap)
	if err != nil || snap.State != execution.Running || snap.Trigger != nil {
		t.Fatal("manual claim treated as automatic", snap, err)
	}
}

func TestScheduledAdmissionRollbackAndRevisionRace(t *testing.T) {
	s, r, f, now, _ := scheduledFixture(t)
	cap := execution.DefaultCapacity()
	entries, err := s.DueSchedules(t.Context(), f, now)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	cron, err := schedule.Compile(entries[0].Trigger.Schedule, "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := cron.Calculate(t.Context(), entries[0].Cursor, now, "catch_up_once", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(t.Context(), "CREATE TRIGGER fail_schedule BEFORE INSERT ON schedule_events BEGIN SELECT RAISE(ABORT,'injected schedule failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdvanceSchedule(t.Context(), f, entries[0], plan, "", cap, now); err == nil {
		t.Fatal("failure hidden")
	}
	for _, table := range []string{"executions", "execution_queue", "scheduled_executions", "schedule_events"} {
		var count int
		if err = s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	current, err := s.Schedules(t.Context(), r.Name)
	if err != nil || current[0].Version != entries[0].Version || !current[0].Cursor.Equal(entries[0].Cursor) {
		t.Fatal(current, err)
	}
	if _, err = s.db.ExecContext(t.Context(), "DROP TRIGGER fail_schedule"); err != nil {
		t.Fatal(err)
	}
	replacement := dispatcherRevision(t, s, []byte(strings.Replace(string(r.Source), "message: hello", "message: updated", 1)))
	if _, err = s.AdvanceSchedule(t.Context(), f, entries[0], plan, "", cap, now); !errors.Is(err, scheduler.ErrStaleEntry) {
		t.Fatal("admitted obsolete schedule", err)
	}
	rows, err := s.Schedules(t.Context(), r.Name)
	if err != nil || rows[0].Revision != replacement.Revision || rows[0].Pending != nil {
		t.Fatal(rows, err)
	}
}

func TestScheduleConcurrentScansHaveOneWinner(t *testing.T) {
	s, r, f, now, path := scheduledFixture(t)
	other := openTest(t, path)
	cap := execution.DefaultCapacity()
	reports := make(chan scheduler.Report, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, store := range []*Store{s, other} {
		wg.Go(func() {
			report, err := scheduleService(store).Tick(t.Context(), f, cap, now)
			reports <- report
			errs <- err
		})
	}
	wg.Wait()
	sum := (<-reports).Admitted + (<-reports).Admitted
	if sum != 1 {
		t.Fatal(sum)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	history, err := s.ScheduleEvents(t.Context(), r.Name, 100)
	encoded, _ := json.Marshal(history)
	if err != nil || len(history.Events) != 1 || strings.Contains(string(encoded), "message") {
		t.Fatal(history, err)
	}
}

func TestScheduleMultipleTriggersAndLifecycleChangeDuringPreflight(t *testing.T) {
	s, r, f, _, path := scheduledFixture(t)
	source := strings.Replace(string(r.Source), "missed_run: catch_up_once}]", "missed_run: catch_up_once}, {id: other, type: cron, schedule: '* * * * *', timezone: UTC}]", 1)
	r = dispatcherRevision(t, s, []byte(source))
	now := r.CreatedAt.Truncate(time.Minute).Add(time.Minute)
	other := openTest(t, path)
	service := scheduleService(s)
	service.Preflight = func(ctx context.Context, _ registry.Record) error {
		_, err := other.SetEnabled(ctx, r.Name, false)
		return err
	}
	cap := execution.DefaultCapacity()
	report, err := service.Tick(t.Context(), f, cap, now)
	if err != nil || report.Admitted != 0 {
		t.Fatal(report, err)
	}
	rows, err := s.Executions(t.Context())
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	enabled, err := other.SetEnabled(t.Context(), r.Name, true)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.Schedules(t.Context(), r.Name)
	if err != nil || len(entries) != 2 || !entries[0].Cursor.Equal(enabled.UpdatedAt) || !entries[1].Cursor.Equal(enabled.UpdatedAt) {
		t.Fatal(entries, enabled, err)
	}
	now = enabled.UpdatedAt.Truncate(time.Minute).Add(time.Minute)
	service = scheduleService(s)
	for range 2 {
		if _, err := service.Tick(t.Context(), f, cap, now); err != nil {
			t.Fatal(err)
		}
	}
	history, err := s.ScheduleEvents(t.Context(), r.Name, 10)
	if err != nil || len(history.Events) != 2 || history.Events[0].Trigger == history.Events[1].Trigger || !history.Events[0].ScheduledAt.Equal(*history.Events[1].ScheduledAt) {
		t.Fatal(history, err)
	}
}

func TestSchedulerDefersUnavailableActionsAndReturnsInfrastructureErrors(t *testing.T) {
	s, r, f, now, _ := scheduledFixture(t)
	service := scheduleService(s)
	cap := execution.DefaultCapacity()
	failure := errors.New("private infrastructure diagnostic")
	service.Preflight = func(context.Context, registry.Record) error { return failure }
	if report, err := service.Tick(t.Context(), f, cap, now); !errors.Is(err, failure) || report.Scanned != 0 {
		t.Fatal(report, err)
	}
	service.Preflight = func(context.Context, registry.Record) error { return execution.ErrUnavailable }
	if report, err := service.Tick(t.Context(), f, cap, now); err != nil || report.Admitted != 0 || report.Deferred != 1 {
		t.Fatal(report, err)
	}
	service.Preflight = func(context.Context, registry.Record) error { return nil }
	if report, err := service.Tick(t.Context(), f, cap, now.Add(25*time.Hour)); err != nil || report.Admitted != 1 {
		t.Fatal(report, err)
	}
	history, err := s.ScheduleEvents(t.Context(), r.Name, 10)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, event := range history.Events {
		codes[event.Code] = true
	}
	for _, code := range []string{"action_unavailable", "intent_expired", "schedule_skipped", "admitted"} {
		if !codes[code] {
			t.Fatal(code, history)
		}
	}
}

func TestMigrationFiveRollbackPreservesOldHistory(t *testing.T) {
	s, r, _ := reliableStoreFixture(t)
	removeMigrationSeven(t, s)
	admission, err := s.CreateReliable(t.Context(), r, execution.AdmissionOptions{Owner: testOwner})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"DROP TABLE event_history", "DROP TABLE event_dedup", "DROP TABLE event_executions", "DROP TABLE startup_scan", "DROP TABLE event_routes", "DROP TABLE schedule_events", "DROP TABLE scheduled_executions", "DROP TABLE schedules", "ALTER TABLE workflows DROP COLUMN automatic_generation", "DELETE FROM schema_migrations WHERE version>=5", "CREATE TABLE schedules(marker TEXT)", "INSERT INTO schedules VALUES('preserved')"} {
		if _, err := s.db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.migrate(t.Context()); err == nil {
		t.Fatal("conflicting schema accepted")
	}
	var count int
	if err = s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM pragma_table_info('workflows') WHERE name='automatic_generation'").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	var marker string
	if err = s.db.QueryRowContext(t.Context(), "SELECT marker FROM schedules").Scan(&marker); err != nil || marker != "preserved" {
		t.Fatal(marker, err)
	}
	if _, err = s.db.ExecContext(t.Context(), "DROP TABLE schedules"); err != nil {
		t.Fatal(err)
	}
	if err = s.migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Execution(t.Context(), admission.Snapshot.ID)
	want, marshalErr := json.Marshal(admission.Snapshot)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	actual, marshalErr := json.Marshal(got)
	if err != nil || marshalErr != nil || string(want) != string(actual) {
		t.Fatal(got, err, marshalErr)
	}
}
