package gateway

import (
	"distributed-job-scheduler/pkg/loadbalancer"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type APIGateway struct {
	lb *loadbalancer.LoadBalancer
}

type Job struct {
	JobId       primitive.ObjectID `json:"job_id" bson:"job_id,omitempty"`
	ScheduledAt time.Time          `json:"scheduled_at" bson:"scheduled_at"`
	CreatedAt   time.Time          `json:"created_at" bson:"created_at"`
	LastRunAt   time.Time          `json:"last_run_at" bson:"last_run_at"`
	NextRunAt   time.Time          `json:"next_run_at" bson:"next_run_at"`
	Status      string             `json:"status" bson:"status"`
	Payload     string             `json:"payload" bson:"payload"`
	RetryCount  int                `json:"retry_count" bson:"retry_count"`
	MaxRetries  int                `json:"max_retries" bson:"max_retries"`
}

type JobExecution struct {
	ExecId       primitive.ObjectID `json:"exec_id" bson:"exec_id,omitempty"`
	JobId        primitive.ObjectID `json:"job_id" bson:"job_id"`
	StartedAt    time.Time          `json:"started_at" bson:"started_at"`
	FinishedAt   *time.Time         `json:"finished_at,omitempty" bson:"finished_at,omitempty"`
	Status       string             `json:"status" bson:"status"`
	ErrorMessage string             `json:"error_message,omitempty" bson:"error_message,omitempty"`
}

type JobResponse struct {
	JobId   primitive.ObjectID `json:"job_id" bson:"job_id"`
	Status  string             `json:"status"`
	Message string             `json:"message"`
}
