package schedule

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCronNumericFieldsAndDaySemantics(t *testing.T) {
	for _, test := range []struct {
		expression, at string
		want           bool
	}{
		{"*/15 9-17/2 * * 1-5", "2026-11-09T09:30:00Z", true},
		{"*/15 9-17/2 * * 1-5", "2026-11-09T10:30:00Z", false},
		{"5/15 0,12 * * *", "2026-11-09T12:35:00Z", true},
		{"0 0 1 * 1", "2026-11-01T00:00:00Z", true},
		{"0 0 1 * 1", "2026-11-09T00:00:00Z", true},
		{"0 0 1 * 1", "2026-11-10T00:00:00Z", false},
		{"0 0 */2 * 1", "2026-11-01T00:00:00Z", false},
		{"0 0 */2 * 1", "2026-11-09T00:00:00Z", true},
	} {
		c, err := Compile(test.expression, "UTC", nil)
		if err != nil {
			t.Fatal(err)
		}
		at, err := time.Parse(time.RFC3339, test.at)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.Matches(at); got != test.want {
			t.Fatal(test, got)
		}
	}
}

func TestCronRejectsMalformedBoundedInput(t *testing.T) {
	for _, expression := range []string{"", "@daily", "0 0 * * * *", "TZ=UTC 0 0 * * *", "0 0 * JAN MON", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 7", "*/0 * * * *", "*/61 * * * *", "-1 * * * *", "1--2 * * * *", "3-1 * * * *", "1/2/3 * * * *", "1, * * * *", "* * * * *\n", strings.Repeat("*", 257), "０ 0 * * *"} {
		if _, err := Compile(expression, "UTC", nil); !errors.Is(err, ErrInvalid) {
			t.Fatal(expression, err)
		}
	}
	for _, zone := range []string{"", "Local", "/etc/passwd", "../UTC", "a/../b", "a//b", "a\\b", "file:UTC", strings.Repeat("a", 129), "Not/AZone"} {
		if _, err := Compile("* * * * *", zone, nil); !errors.Is(err, ErrInvalid) {
			t.Fatal(zone, err)
		}
	}
	for _, load := range []ZoneLoader{func(string) (*time.Location, error) { return nil, nil }, func(string) (*time.Location, error) { return nil, errors.New("private-zone-error") }} {
		if _, err := Compile("* * * * *", "UTC", load); err != ErrInvalid {
			t.Fatal(err)
		}
	}
}

func TestCronDSTAndLeapDayUTCIdentities(t *testing.T) {
	spring, err := Compile("30 2 * * *", "America/New_York", nil)
	if err != nil {
		t.Fatal(err)
	}
	cursor := time.Date(2026, 3, 8, 6, 0, 0, 0, time.UTC)
	now := cursor.Add(2 * time.Hour)
	plan, err := spring.Calculate(t.Context(), cursor, now, "catch_up_once", nil)
	if err != nil || plan.Due != nil {
		t.Fatal(plan, err)
	}
	fall, err := Compile("30 1 * * *", "America/New_York", nil)
	if err != nil {
		t.Fatal(err)
	}
	cursor = time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)
	first, err := fall.Calculate(t.Context(), cursor, cursor.Add(90*time.Minute), "skip", nil)
	if err != nil || first.Due == nil || first.Due.Hour() != 5 {
		t.Fatal(first, err)
	}
	second, err := fall.Calculate(t.Context(), first.Cursor, cursor.Add(150*time.Minute), "skip", nil)
	if err != nil || second.Due == nil || second.Due.Hour() != 6 || first.Due.Equal(*second.Due) {
		t.Fatal(second, err)
	}
	coalesced, err := fall.Calculate(t.Context(), cursor, cursor.Add(3*time.Hour), "catch_up_once", nil)
	if err != nil || coalesced.Due == nil || coalesced.Due.Hour() != 6 || coalesced.Skipped != 1 {
		t.Fatal(coalesced, err)
	}
	leap, err := Compile("0 0 29 2 *", "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	now = time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	plan, err = leap.Calculate(t.Context(), now.Add(-time.Hour), now, "skip", nil)
	if err != nil || plan.Due == nil || !plan.Due.Equal(now) {
		t.Fatal(plan, err)
	}
	impossible, err := Compile("0 0 31 2 *", "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = impossible.Calculate(t.Context(), now.Add(-365*24*time.Hour), now, "catch_up_once", nil)
	if err != nil || plan.Due != nil {
		t.Fatal(plan, err)
	}
}

func TestCronCursorPendingAndHorizon(t *testing.T) {
	c, err := Compile("* * * * *", "UTC", nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 8, 12, 0, 30, 0, time.UTC)
	cursor := now.Add(-48 * time.Hour)
	plan, err := c.Calculate(t.Context(), cursor, now, "catch_up_once", nil)
	if err != nil || plan.Due == nil || !plan.Due.Equal(now.Truncate(time.Minute)) || plan.Skipped > 1440 {
		t.Fatal(plan, err)
	}
	pending := now.Add(-time.Hour).Truncate(time.Minute)
	plan, err = c.Calculate(t.Context(), now.Add(-time.Minute), now, "skip", &pending)
	if err != nil || !plan.Due.Equal(pending) {
		t.Fatal(plan, err)
	}
	expired := now.Add(-25 * time.Hour)
	plan, err = c.Calculate(t.Context(), cursor, now, "skip", &expired)
	if err != nil || !plan.Expired || plan.Due == nil {
		t.Fatal(plan, err)
	}
	plan, err = c.Calculate(t.Context(), now, now.Add(-time.Hour), "skip", nil)
	if err != nil || !plan.Cursor.Equal(now) || plan.Due != nil {
		t.Fatal(plan, err)
	}
	future := now.Add(time.Minute)
	plan, err = c.Calculate(t.Context(), cursor, now, "skip", &future)
	if err != nil || plan.Due != nil || !plan.Cursor.Equal(cursor) {
		t.Fatal(plan, err)
	}
	sameMinute := now.Add(-time.Second)
	plan, err = c.Calculate(t.Context(), sameMinute, now, "skip", nil)
	if err != nil || plan.Due != nil {
		t.Fatal("backfilled before first deployment", plan, err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = c.Calculate(cancelled, cursor, now, "skip", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, policy := range []string{"unknown", ""} {
		if _, err = c.Calculate(t.Context(), cursor, now, policy, nil); err == nil {
			t.Fatal(policy)
		}
	}
	if _, err = c.Calculate(t.Context(), time.Time{}, now, "skip", nil); err == nil {
		t.Fatal("zero cursor accepted")
	}
}

func FuzzCron(f *testing.F) {
	for _, source := range []string{"* * * * *", "*/15 1-3 * * 0,6", "0 0 29 2 *", "@daily", "*/0 * * * *"} {
		f.Add(source, "UTC")
	}
	f.Fuzz(func(t *testing.T, source, zone string) {
		c, err := Compile(source, zone, func(name string) (*time.Location, error) {
			if name != "UTC" {
				return nil, ErrInvalid
			}
			return time.UTC, nil
		})
		if err != nil {
			return
		}
		now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
		plan, err := c.Calculate(t.Context(), now.Add(-100*24*time.Hour), now, "catch_up_once", nil)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Due != nil && (!c.Matches(*plan.Due) || plan.Due.After(now) || now.Sub(*plan.Due) > 24*time.Hour) {
			t.Fatal(plan)
		}
	})
}

func BenchmarkCronScan(b *testing.B) {
	c, err := Compile("17 2 * * 1", "America/New_York", nil)
	if err != nil {
		b.Fatal(err)
	}
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Calculate(b.Context(), now.Add(-48*time.Hour), now, "catch_up_once", nil); err != nil {
			b.Fatal(err)
		}
	}
}
