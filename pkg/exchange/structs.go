package exchange

import (
	"sync"
)

type Exchange struct {
	port                string
	exchangeID          string
	normalQueuePorts    []string
	highQueuePorts      []string
	healthyNormalQueues []string
	healthyHighQueues   []string
	queueHealthMu       sync.RWMutex
	stats               *ExchangeStats
	stopChan            chan struct{}
	wg                  sync.WaitGroup
}

type ExchangeStats struct {
	mu            sync.RWMutex
	JobsRouted    int64
	RoutingErrors int64
}
