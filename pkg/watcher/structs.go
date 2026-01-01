package watcher

import (
	"distributed-job-scheduler/pkg/queue"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

type Watcher struct {
	client      *mongo.Client
	db          *mongo.Database
	queue       *queue.Queue
	watcherId   string
	port        string
	stats       *WatcherStats
	wg          sync.WaitGroup
	stopChannel chan struct{}
}

type WatcherStats struct {
	mu               sync.RWMutex
	JobsQueuedCount  int
	JobsClaimedCount int
	LastClaimedTime  time.Time // when picked from db
	LastQueuedTime   time.Time // when pushed to queue
}
