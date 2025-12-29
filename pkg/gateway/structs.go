package gateway

import (
	"distributed-job-scheduler/pkg/loadbalancer"
	"time"
)

type APIGateway struct {
	lb *loadbalancer.LoadBalancer
}

type Job struct {
	Id          string    `json:"id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Payload     string    `json:"payload"`
}

type JobResponse struct {
	JobId   string `json:"job_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
