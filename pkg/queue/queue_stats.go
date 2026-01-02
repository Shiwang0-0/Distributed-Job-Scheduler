package queue

import "time"

func (qs *QueueStats) IncrementPushed() {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.JobsPushed++
	qs.LastPushTime = time.Now()
}

func (qs *QueueStats) IncrementPopped() {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	qs.JobsPopped++
	qs.LastPopTime = time.Now()
}

func (qs *QueueStats) Get() map[string]interface{} {
	qs.mu.RLock()
	defer qs.mu.RUnlock()

	return map[string]interface{}{
		"jobs_pushed":    qs.JobsPushed,
		"jobs_popped":    qs.JobsPopped,
		"jobs_recovered": qs.JobsRecovered,
		"last_push_time": qs.LastPushTime,
		"last_pop_time":  qs.LastPopTime,
	}
}
