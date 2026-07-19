package entity

import "time"

// JobStatus is the lifecycle state of a background job.
type JobStatus string

const (
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
)

// JobKind identifies what a background job does.
type JobKind string

const (
	JobKindSendTest     JobKind = "send_test"
	JobKindScheduleSend JobKind = "schedule_send"
	JobKindSync         JobKind = "sync"
)

// Job is one background delivery run tracked in memory. It is returned to the
// admin UI so a long-running trigger can be started without blocking the caller.
type Job struct {
	ID         string         `json:"id"`
	Kind       JobKind        `json:"kind"`
	Label      string         `json:"label"`
	Status     JobStatus      `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Error      string         `json:"error,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
}
