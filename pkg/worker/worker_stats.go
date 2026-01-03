package worker

func (ws *WorkerStats) IncrementRunningCount() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.CurrentlyRunning++
}

func (ws *WorkerStats) DecrementRunningCount() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.CurrentlyRunning--
}

func (ws *WorkerStats) IncrementSuccessCount() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.JobsSucceeded++
}

func (ws *WorkerStats) IncrementFailureCount() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.JobsFailed++
}

func (ws *WorkerStats) GetStats() map[string]interface{} {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return map[string]interface{}{
		"jobs_succeeded":      ws.JobsSucceeded,
		"jobs_failed":         ws.JobsFailed,
		"last_execution_time": ws.LastExecutionTime,
		"currently_running":   ws.CurrentlyRunning,
	}
}
