package workerqueuecoordinator

import (
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

const (
	HeartbeatInterval = 10 * time.Second
	HeartbeatTimeout  = 30 * time.Second
	LeaseDuration     = 60 * time.Second
)

type Coordinator struct {
	client        *mongo.Client
	db            *mongo.Database
	coordinatorID string
	port          string
	allQueuePorts []string // All available queue ports
	stats         *CoordinatorStats
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

type WorkerAssignment struct {
	WorkerID       string    `bson:"worker_id" json:"worker_id"`
	QueuePort      string    `bson:"queue_port" json:"queue_port"`
	AssignedAt     time.Time `bson:"assigned_at" json:"assigned_at"`
	LastHeartbeat  time.Time `bson:"last_heartbeat" json:"last_heartbeat"`
	LeaseExpiresAt time.Time `bson:"lease_expires_at" json:"lease_expires_at"`
	Status         string    `bson:"status" json:"status"` // "active", "stale"
}

type CoordinatorStats struct {
	mu                    sync.RWMutex
	ActiveWorkers         int
	TotalAssignments      int64
	StaleWorkersRecovered int64
	LastAssignmentTime    time.Time
}
