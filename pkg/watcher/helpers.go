package watcher

import "time"

func (ws *WatcherStats) IncrementClaimedJobCount() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.JobsClaimedCount++
	ws.LastClaimedTime = time.Now()
}

func (ws *WatcherStats) IncrementQueuedJobCount() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.JobsQueuedCount++
	ws.LastQueuedTime = time.Now()
}

func (ws *WatcherStats) GetStats() map[string]interface{} {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return map[string]interface{}{
		"jobs_claimed":     ws.JobsClaimedCount,
		"jobs_queued":      ws.JobsQueuedCount,
		"last_claim_time":  ws.LastClaimedTime,
		"last_queued_time": ws.LastQueuedTime,
	}
}
