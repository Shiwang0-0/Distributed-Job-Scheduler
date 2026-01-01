package queue

import (
	"distributed-job-scheduler/pkg/gateway"
	"sync"
)

type Queue struct {
	Items []QueueItem
	Mu    sync.RWMutex
}

type QueueItem struct {
	Job gateway.Job
}
