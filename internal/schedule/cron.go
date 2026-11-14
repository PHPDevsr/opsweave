// Package schedule defines bounded cron syntax and UTC occurrence calculations.
package schedule

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Keep explicit IANA zones available on every release platform.
)

// ErrInvalid never includes a supplied expression, timezone, or payload.
var ErrInvalid = errors.New("invalid cron schedule or timezone")

// Trigger is the reviewed cron-only workflow trigger contract.
type Trigger struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Schedule  string `json:"schedule"`
	Timezone  string `json:"timezone"`
	MissedRun string `json:"missed_run"`
}

// ZoneLoader allows deterministic calendar/availability tests. Nil uses embedded-capable Go lookup.
type ZoneLoader func(string) (*time.Location, error)

// Cron is immutable and safe for concurrent matching.
type Cron struct {
	fields            [5]uint64
	dayWild, weekWild bool
	zone              *time.Location
}

// Compile accepts numeric five-field cron only, with bounded lists/ranges/positive steps.
func Compile(expression, zone string, load ZoneLoader) (*Cron, error) {
	if len(expression) == 0 || len(expression) > 256 || !validZone(zone) {
		return nil, ErrInvalid
	}
	for _, b := range []byte(expression) {
		if b != ' ' && b != '\t' && (b < '!' || b > '~') {
			return nil, ErrInvalid
		}
	}
	parts := strings.Fields(expression)
	if len(parts) != 5 {
		return nil, ErrInvalid
	}
	c := &Cron{}
	bounds := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	for i, p := range parts {
		bits, wild, err := parseField(p, bounds[i][0], bounds[i][1])
		if err != nil {
			return nil, err
		}
		c.fields[i] = bits
		if i == 2 {
			c.dayWild = wild
		}
		if i == 4 {
			c.weekWild = wild
		}
	}
	if load == nil {
		load = time.LoadLocation
	}
	var err error
	c.zone, err = load(zone)
	if err != nil || c.zone == nil {
		return nil, ErrInvalid
	}
	return c, nil
}

func validZone(zone string) bool {
	if zone == "" || len(zone) > 128 || zone == "Local" || strings.HasPrefix(zone, "/") {
		return false
	}
	for _, part := range strings.Split(zone, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	for _, b := range []byte(zone) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("/_+-", rune(b))) {
			return false
		}
	}
	return true
}

func number(s string, min, max int) (int, error) {
	if len(s) == 0 || len(s) > 3 {
		return 0, ErrInvalid
	}
	for _, b := range []byte(s) {
		if b < '0' || b > '9' {
			return 0, ErrInvalid
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < min || n > max {
		return 0, ErrInvalid
	}
	return n, nil
}

func parseField(source string, min, max int) (uint64, bool, error) {
	var bits uint64
	wild := false
	for _, part := range strings.Split(source, ",") {
		terms := strings.Split(part, "/")
		if len(terms) > 2 {
			return 0, false, ErrInvalid
		}
		step := 1
		var err error
		if len(terms) == 2 {
			step, err = number(terms[1], 1, max-min+1)
			if err != nil {
				return 0, false, err
			}
		}
		start, end := min, max
		if terms[0] == "*" {
			wild = true
		} else {
			ends := strings.Split(terms[0], "-")
			if len(ends) > 2 {
				return 0, false, ErrInvalid
			}
			start, err = number(ends[0], min, max)
			if err != nil {
				return 0, false, err
			}
			end = start
			if len(ends) == 2 {
				end, err = number(ends[1], start, max)
				if err != nil {
					return 0, false, err
				}
			} else if len(terms) == 2 {
				end = max
			}
		}
		for v := start; v <= end; v += step {
			bits |= uint64(1) << v
		}
	}
	if bits == 0 {
		return 0, false, ErrInvalid
	}
	return bits, wild, nil
}

// Matches tests local calendar fields at a UTC instant. Repeated DST minutes remain distinct instants.
func (c *Cron) Matches(at time.Time) bool {
	local := at.In(c.zone)
	values := [5]int{local.Minute(), local.Hour(), local.Day(), int(local.Month()), int(local.Weekday())}
	matches := [5]bool{}
	for i, v := range values {
		matches[i] = c.fields[i]&(uint64(1)<<v) != 0
	}
	day := matches[2] && matches[4]
	if !c.dayWild && !c.weekWild {
		day = matches[2] || matches[4]
	}
	return matches[0] && matches[1] && matches[3] && day
}

// Plan contains one bounded scan result. A pending intent is retried without changing its identity.
type Plan struct {
	Cursor  time.Time
	Due     *time.Time
	Skipped int
	Expired bool
}

// Calculate advances a monotonic cursor and chooses at most one occurrence in the last 24 hours.
// skip admits the current minute only; catch_up_once selects the newest matching missed minute.
func (c *Cron) Calculate(ctx context.Context, cursor, now time.Time, policy string, pending *time.Time) (Plan, error) {
	result := Plan{Cursor: cursor.UTC()}
	now = now.UTC()
	if policy != "skip" && policy != "catch_up_once" || cursor.IsZero() || now.IsZero() || now.Year() < 1 || now.Year() > 9999 {
		return result, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if now.Before(cursor) {
		return result, nil
	}
	result.Cursor = now
	if pending != nil {
		if pending.After(now) {
			return Plan{Cursor: cursor.UTC()}, nil
		}
		if now.Sub(*pending) <= 24*time.Hour {
			due := pending.UTC()
			result.Due = &due
			return result, nil
		}
		result.Expired = true
	}
	lower := cursor.Truncate(time.Minute).Add(time.Minute)
	horizon := now.Add(-24 * time.Hour)
	if lower.Before(horizon) {
		lower = horizon.Truncate(time.Minute)
		if lower.Before(horizon) {
			lower = lower.Add(time.Minute)
		}
	}
	for at := now.Truncate(time.Minute); !at.Before(lower); at = at.Add(-time.Minute) {
		if err := ctx.Err(); err != nil {
			return Plan{}, err
		}
		if !c.Matches(at) {
			continue
		}
		if result.Due == nil && (policy == "catch_up_once" || at.Equal(now.Truncate(time.Minute))) {
			due := at
			result.Due = &due
		} else {
			result.Skipped++
		}
	}
	return result, nil
}
