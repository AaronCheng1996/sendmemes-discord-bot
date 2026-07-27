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
	"strconv"
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

// Describe returns a short human-readable summary of s (e.g. "every 6h" or
// "at 09:00 every day"). It covers common duration and cron shapes; anything
// it doesn't recognize is returned verbatim rather than guessed at.
func Describe(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}

	if d, err := time.ParseDuration(trimmed); err == nil && d > 0 {
		return "every " + trimmed
	}

	switch trimmed {
	case "@hourly":
		return "every hour"
	case "@daily", "@midnight":
		return "at 00:00 every day"
	case "@weekly":
		return "weekly"
	case "@monthly":
		return "monthly"
	case "@yearly", "@annually":
		return "yearly"
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 5 {
		if desc, ok := describeCronFields(fields); ok {
			return desc
		}
	}

	return trimmed
}

// describeCronFields recognizes a handful of common 5-field cron shapes
// (fixed minute/hour with every day, or every day-of-week). It returns
// ok=false for anything more complex, leaving the caller to fall back to the
// raw expression.
func describeCronFields(fields []string) (string, bool) {
	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	if dom == "*" && month == "*" {
		if m, herr := strconv.Atoi(minute); herr == nil {
			if h, herr2 := strconv.Atoi(hour); herr2 == nil {
				clock := fmt.Sprintf("%02d:%02d", h, m)
				if dow == "*" {
					return "at " + clock + " every day", true
				}
				if name, ok := weekdayName(dow); ok {
					return "at " + clock + " every " + name, true
				}
			}
		}
	}

	if minute == "0" && dom == "*" && month == "*" && dow == "*" {
		if rest := strings.TrimPrefix(hour, "*/"); rest != hour {
			if n, herr := strconv.Atoi(rest); herr == nil {
				return fmt.Sprintf("every %d hours", n), true
			}
		}
	}

	if hour == "*" && dom == "*" && month == "*" && dow == "*" {
		if rest := strings.TrimPrefix(minute, "*/"); rest != minute {
			if n, merr := strconv.Atoi(rest); merr == nil {
				return fmt.Sprintf("every %d minutes", n), true
			}
		}
	}

	return "", false
}

var cronWeekdays = map[string]string{
	"0": "Sunday", "1": "Monday", "2": "Tuesday", "3": "Wednesday",
	"4": "Thursday", "5": "Friday", "6": "Saturday", "7": "Sunday",
	"sun": "Sunday", "mon": "Monday", "tue": "Tuesday", "wed": "Wednesday",
	"thu": "Thursday", "fri": "Friday", "sat": "Saturday",
}

func weekdayName(dow string) (string, bool) {
	name, ok := cronWeekdays[strings.ToLower(dow)]
	return name, ok
}
