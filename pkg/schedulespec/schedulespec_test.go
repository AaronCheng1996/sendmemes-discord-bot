package schedulespec

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "duration hours", input: "6h", wantErr: false},
		{name: "duration composite", input: "1h30m", wantErr: false},
		{name: "duration minutes", input: "90m", wantErr: false},
		{name: "cron five fields", input: "0 9 * * *", wantErr: false},
		{name: "cron step", input: "*/15 * * * *", wantErr: false},
		{name: "cron descriptor daily", input: "@daily", wantErr: false},
		{name: "cron descriptor hourly", input: "@hourly", wantErr: false},
		{name: "whitespace trimmed", input: "  0 0 * * *  ", wantErr: false},
		{name: "zero duration", input: "0s", wantErr: true},
		{name: "negative duration", input: "-5m", wantErr: true},
		{name: "empty", input: "", wantErr: true},
		{name: "blank", input: "   ", wantErr: true},
		{name: "garbage", input: "not-a-schedule", wantErr: true},
		{name: "too few cron fields", input: "0 9 * *", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = nil error, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if spec == nil {
				t.Fatalf("Parse(%q) returned nil spec", tt.input)
			}
		})
	}
}

func TestIntervalSpecNext(t *testing.T) {
	spec, err := Parse("6h")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	base := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	got := spec.Next(base)
	want := base.Add(6 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("interval Next = %v, want %v", got, want)
	}
	// Next is always measured from the argument, so a later call moves forward.
	got2 := spec.Next(got)
	if !got2.Equal(want.Add(6 * time.Hour)) {
		t.Fatalf("interval Next(second) = %v, want %v", got2, want.Add(6*time.Hour))
	}
}

func TestCronSpecNext(t *testing.T) {
	spec, err := Parse("0 9 * * *") // every day at 09:00
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// From 08:00 the next fire is the same day at 09:00.
	base := time.Date(2026, 1, 2, 8, 0, 0, 0, time.UTC)
	got := spec.Next(base)
	want := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("cron Next = %v, want %v", got, want)
	}
	// From 09:30 it rolls to the next day.
	base2 := time.Date(2026, 1, 2, 9, 30, 0, 0, time.UTC)
	got2 := spec.Next(base2)
	want2 := time.Date(2026, 1, 3, 9, 0, 0, 0, time.UTC)
	if !got2.Equal(want2) {
		t.Fatalf("cron Next(next day) = %v, want %v", got2, want2)
	}
}
