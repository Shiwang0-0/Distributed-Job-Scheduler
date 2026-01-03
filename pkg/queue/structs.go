package queue

import (
	"distributed-job-scheduler/pkg/gateway"
	"sync"
	"time"
)

type Queue struct {
	items []*QueueItem
	mu    sync.RWMutex
}

type QueueItem struct {
	Job       gateway.Job
	VisibleAt time.Time // visible timeout
	ClaimedBy string
	ClaimedAt time.Time
}

type QueueService struct {
	queue    *Queue
	queueID  string
	port     string
	stopChan chan struct{}
	wg       sync.WaitGroup
	stats    *QueueStats
}

type QueueStats struct {
	mu            sync.RWMutex
	JobsPushed    int64
	JobsPopped    int64
	JobsRecovered int64
	LastPushTime  time.Time
	LastPopTime   time.Time
}
