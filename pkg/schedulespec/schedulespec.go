// Package schedulespec parses a schedule string into a Spec that can yield the
// next fire time. Two syntaxes are accepted:
//
//   - a Go duration (e.g. "6h", "90m", "1h30m") — fires at a fixed interval
//     measured from the moment Next is called;
//   - a standard 5-field cron expression or descriptor (e.g. "0 9 * * *",
//     "*/15 * * * *", "@daily") — fires at wall-clock times.
//
// Duration is tried first, so bare numbers with a unit never fall through to
// the cron parser.
package schedulespec

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Spec yields the next fire time strictly after the given instant.
type Spec interface {
	// Next returns the next fire time after t. A zero time means "never".
	Next(after time.Time) time.Time
}

// intervalSpec fires every d, measured from the call to Next.
type intervalSpec struct {
	d time.Duration
}

func (s intervalSpec) Next(after time.Time) time.Time {
	return after.Add(s.d)
}

// cronSpec wraps a parsed cron schedule (wall-clock based).
type cronSpec struct {
	sched cron.Schedule
}

func (s cronSpec) Next(after time.Time) time.Time {
	return s.sched.Next(after)
}

// Parse interprets s as a Go duration first, then as a standard cron
// expression. It returns an error naming both accepted formats when neither
// matches.
func Parse(s string) (Spec, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, fmt.Errorf("schedule is empty: use a Go duration (e.g. 6h) or a cron expression (e.g. 0 9 * * *)")
	}

	// Duration first: a valid, positive duration wins outright.
	if d, err := time.ParseDuration(trimmed); err == nil {
		if d <= 0 {
			return nil, fmt.Errorf("schedule duration %q must be positive", trimmed)
		}
		return intervalSpec{d: d}, nil
	}

	// Otherwise try a standard 5-field cron expression (also accepts
	// descriptors like @daily / @hourly).
	if sched, err := cron.ParseStandard(trimmed); err == nil {
		return cronSpec{sched: sched}, nil
	}

	return nil, fmt.Errorf(
		"invalid schedule %q: use a Go duration (e.g. 6h, 90m) or a cron expression (e.g. 0 9 * * *, @daily)",
		trimmed,
	)
}
