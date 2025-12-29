package loadbalancer

import "sync"

type SchedulerService struct {
	Url     string
	Healthy bool
	mu      sync.RWMutex
}

type LoadBalancer struct {
	Services []*SchedulerService
	Current  int // to know which service will run next
	mu       sync.Mutex
}
