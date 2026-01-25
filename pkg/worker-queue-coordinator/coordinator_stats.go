package workerqueuecoordinator

import "time"

func (c *CoordinatorStats) IncrementAssignments() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.TotalAssignments++
	c.LastAssignmentTime = time.Now()
}

func (c *CoordinatorStats) IncrementStaleRecovered(count int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.StaleWorkersRecovered += count
}

func (c *CoordinatorStats) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"total_assignments":       c.TotalAssignments,
		"stale_workers_recovered": c.StaleWorkersRecovered,
		"last_assignment_time":    c.LastAssignmentTime,
	}
}
