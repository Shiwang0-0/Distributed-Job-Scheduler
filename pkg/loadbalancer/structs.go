package loadbalancer

import "sync"

type SchedulerNode struct {
	Url     string
	Healthy bool
	mu      sync.RWMutex
}

type LoadBalancer struct {
	Services []*SchedulerNode
	Current  int // to know which service will run next
	mu       sync.Mutex
}
