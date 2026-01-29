package worker

import (
	jobregistry "distributed-job-scheduler/pkg/job-registry"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

type Worker struct {
	client            *mongo.Client
	db                *mongo.Database
	workerId          string
	port              string
	coordinatorPort   string
	assignedQueuePort string
	stopChannel       chan struct{}
	wg                sync.WaitGroup
	stats             *WorkerStats
	concurrency       int
	semaphore         chan struct{}
	jobRegistry       *jobregistry.JobRegistry
}

type WorkerStats struct {
	mu                sync.RWMutex
	JobsSucceeded     int
	JobsFailed        int
	LastExecutionTime time.Time
	CurrentlyRunning  int
}
