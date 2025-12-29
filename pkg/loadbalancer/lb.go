package loadbalancer

import (
	"log"
	"net/http"
	"time"
)

func NewLoadBalancer(serviceURLs []string) *LoadBalancer {

	// initialize the load balancer scheduler service
	lb := &LoadBalancer{
		Services: make([]*SchedulerService, len(serviceURLs)),
	}

	for i, url := range serviceURLs {
		lb.Services[i] = &SchedulerService{Url: url, Healthy: true}
	}

	// checks health asynchronously
	go lb.healthCheck()

	return lb
}

func (lb *LoadBalancer) healthCheck() {
	// runs every 10 second (signal is sent on ticker.C)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, service := range lb.Services {
			// for the current service start a go routine to check its health
			go func(s *SchedulerService) {
				client := &http.Client{Timeout: 5 * time.Second}
				res, err := client.Get(s.Url + "/health")

				s.mu.Lock()
				defer s.mu.Unlock()

				if err != nil || res.StatusCode != http.StatusOK {
					s.Healthy = false
					log.Printf("Service %s is unhealthy", s.Url)
				} else {
					s.Healthy = true
					log.Printf("Service %s is healthy", s.Url)
				}

				if res != nil {
					res.Body.Close()
				}

			}(service)
		}
	}
}

// select a scheduler using round robin
func (lb *LoadBalancer) GetScheduler() *SchedulerService {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	i := 0
	maxAttempts := len(lb.Services)

	for i < maxAttempts {
		service := lb.Services[lb.Current]
		lb.Current = (lb.Current + 1) % maxAttempts

		service.mu.Lock()
		healthy := service.Healthy
		service.mu.Unlock()

		if healthy {
			return service
		}
		i++
	}
	return nil
}
