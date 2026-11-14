// Package scheduler admits persisted cron intents through a fenced repository.
// The dispatcher owns polling; the service creates no goroutines or independent workers.
package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/PHPDevsr/opsweave/internal/execution"
	"github.com/PHPDevsr/opsweave/internal/registry"
	"github.com/PHPDevsr/opsweave/internal/schedule"
)

// ErrStaleEntry indicates a revision, lifecycle, or cursor change since the scan.
var ErrStaleEntry = errors.New("schedule changed during scan")

// Entry is a bounded current cursor and its exact pinned revision.
type Entry struct {
	Workflow    string           `json:"workflow"`
	Revision    string           `json:"revision"`
	Trigger     schedule.Trigger `json:"trigger"`
	Cursor      time.Time        `json:"cursor"`
	Pending     *time.Time       `json:"pending,omitempty"`
	Version     int64            `json:"version"`
	Enabled     bool             `json:"enabled"`
	RetryAt     time.Time        `json:"retryAt"`
	LastFailure string           `json:"lastFailure,omitempty"`
	Record      registry.Record  `json:"-"`
}

// Event is engine-owned schedule history, without payloads, raw errors or secrets.
type Event struct {
	Sequence    int64      `json:"sequence"`
	Workflow    string     `json:"workflow"`
	Revision    string     `json:"revision"`
	Trigger     string     `json:"trigger"`
	Time        time.Time  `json:"time"`
	ScheduledAt *time.Time `json:"scheduledAt,omitempty"`
	Code        string     `json:"code"`
	Execution   string     `json:"execution,omitempty"`
	Skipped     int        `json:"skipped,omitempty"`
}

// History contains the newest bounded events, ordered newest first.
type History struct {
	Events  []Event `json:"events"`
	HasMore bool    `json:"hasMore"`
}

// Result distinguishes admission, deferral and a cursor-only check.
type Result struct {
	Execution string
	Code      string
}

// Repository commits cursor, pending intent, history and queue admission atomically.
type Repository interface {
	DueSchedules(context.Context, execution.Fence, time.Time) ([]Entry, error)
	AdvanceSchedule(context.Context, execution.Fence, Entry, schedule.Plan, string, execution.Capacity, time.Time) (Result, error)
}

// Report makes capacity/availability deferrals observable without claiming successful delivery.
type Report struct{ Scanned, Admitted, Deferred int }

// Service runs one bounded scan. Preflight must validate exact action bindings without invoking work.
type Service struct {
	Repository Repository
	Preflight  func(context.Context, registry.Record) error
	LoadZone   schedule.ZoneLoader
}

// Tick handles at most 64 cursors. Infrastructure errors stop the scan; cursor/identity remain durable.
func (s *Service) Tick(ctx context.Context, fence execution.Fence, capacity execution.Capacity, now time.Time) (report Report, err error) {
	if s == nil || s.Repository == nil || s.Preflight == nil || capacity.Validate() != nil {
		return report, execution.ErrInvalid
	}
	if now.IsZero() || now.Year() < 1 || now.Year() > 9999 {
		return report, execution.ErrInvalid
	}
	entries, err := s.Repository.DueSchedules(ctx, fence, now)
	if err != nil {
		return report, err
	}
	if len(entries) > 64 {
		return report, execution.ErrLimit
	}
	scanDeadline := time.Now().Add(time.Second)
	for index, entry := range entries {
		if index > 0 && !time.Now().Before(scanDeadline) {
			break
		}
		if err = ctx.Err(); err != nil {
			return report, err
		}
		cron, err := schedule.Compile(entry.Trigger.Schedule, entry.Trigger.Timezone, s.LoadZone)
		if err != nil {
			return report, err
		}
		plan, err := cron.Calculate(ctx, entry.Cursor, now, entry.Trigger.MissedRun, entry.Pending)
		if err != nil {
			return report, err
		}
		if plan.Cursor.After(now) {
			continue
		}
		reason := ""
		if plan.Due != nil {
			if err = s.Preflight(ctx, entry.Record); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return report, err
				}
				if !errors.Is(err, execution.ErrUnavailable) && !errors.Is(err, execution.ErrPermission) && !errors.Is(err, registry.ErrArchived) && !errors.Is(err, registry.ErrNotFound) {
					return report, err
				}
				reason = "action_unavailable"
			}
		}
		result, err := s.Repository.AdvanceSchedule(ctx, fence, entry, plan, reason, capacity, now)
		if errors.Is(err, ErrStaleEntry) {
			continue
		}
		if err != nil {
			return report, err
		}
		report.Scanned++
		if result.Code == "admitted" {
			report.Admitted++
		}
		if result.Code == "queue_full" || result.Code == "action_unavailable" {
			report.Deferred++
		}
	}
	return report, nil
}
