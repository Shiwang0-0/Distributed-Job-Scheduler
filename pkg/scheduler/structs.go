package scheduler

import (
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

type Scheduler struct {
	client     *mongo.Client
	db         *mongo.Database
	port       string
	instanceId string
	cronMutex  sync.RWMutex
	stopChan   chan struct{}
	wg         sync.WaitGroup // 1, to ensure that only one scheduler instance does the job

	isLeader     bool
	leaderMu     sync.RWMutex
	leaderTerm   int64         // Monotonic counter for leadership changes
	cronStopChan chan struct{} // Separate channel for cron ticker
	cronTickerMu sync.Mutex
}

const (
	LeaseDuration    = 30 * time.Second
	RenewInterval    = 10 * time.Second
	LeaderCheckDelay = 5 * time.Second
)

type LeaderLease struct {
	ID         string    `bson:"_id"`
	LeaderId   string    `bson:"leader_id"`
	AcquiredAt time.Time `bson:"acquired_at"`
	ExpiresAt  time.Time `bson:"expires_at"`
	UpdatedAt  time.Time `bson:"updated_at"`
}
