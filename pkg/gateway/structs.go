package gateway

import (
	"distributed-job-scheduler/pkg/loadbalancer"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type APIGateway struct {
	lb *loadbalancer.LoadBalancer
}

type JobType string

const (
	JobTypeOnce JobType = "once"
	JobTypeCron JobType = "cron"
)

type JobPriority string

const (
	PriorityHigh   JobPriority = "high"
	PriorityNormal JobPriority = "normal"
)

type Job struct {
	JobId       primitive.ObjectID `json:"job_id" bson:"_id,omitempty"` // bson needs to be _id only
	Type        JobType            `bson:"type" json:"type"`
	Priority    JobPriority        `json:"priority" bson:"priority"`
	ScheduledAt time.Time          `json:"scheduled_at" bson:"scheduled_at"`
	CreatedAt   time.Time          `json:"created_at" bson:"created_at"`
	LastRunAt   time.Time          `json:"last_run_at" bson:"last_run_at"`
	NextRunAt   time.Time          `json:"next_run_at" bson:"next_run_at"`
	UpdatedAt   time.Time          `json:"updated_at" bson:"updated_at"`
	ClaimedBy   string             `bson:"claimed_by,omitempty" json:"claimed_by,omitempty"`
	ClaimedAt   *time.Time         `bson:"claimed_at,omitempty" json:"claimed_at,omitempty"`
	QueuedAt    time.Time          `json:"queued_at" bson:"queued_at"`
	Status      string             `json:"status" bson:"status"`
	Payload     string             `json:"payload" bson:"payload"`
	RetryCount  int                `json:"retry_count" bson:"retry_count"`
	MaxRetries  int                `json:"max_retries" bson:"max_retries"`
	RetryAfter  *time.Time         `json:"retry_after,omitempty" bson:"retry_after,omitempty"`
	LockedBy    string             `json:"locked_by" bson:"locked_by"`
	LockedUntil time.Time          `json:"locked_until" bson:"locked_until"`

	// cron job
	CronExpr string `json:"cron_expr,omitempty" bson:"cron_expr,omitempty"`
}

type JobExecution struct {
	JobId        primitive.ObjectID `json:"job_id" bson:"job_id"`
	WorkerId     string             `json:"worker_id" bson:"worker_id,omitempty"`
	StartedAt    time.Time          `json:"started_at" bson:"started_at"`
	FinishedAt   *time.Time         `json:"finished_at,omitempty" bson:"finished_at,omitempty"`
	Status       string             `json:"status" bson:"status"`
	ErrorMessage string             `json:"error_message,omitempty" bson:"error_message,omitempty"`
}

type JobResponse struct {
	JobId   string `json:"job_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
