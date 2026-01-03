package queue

import "time"

func NewQueue() *Queue {
	return &Queue{
		items: make([]*QueueItem, 0),
	}
}

func (q *Queue) Push(item *QueueItem) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = append(q.items, item)

}

func (q *Queue) Pop() *QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}
	item := q.items[0]
	q.items = q.items[1:]

	return item
}

func (q *Queue) Peek() *QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.items) == 0 {
		return nil
	}

	return q.items[0]
}

func (q *Queue) GetAll() []*QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	items := make([]*QueueItem, len(q.items))
	copy(items, q.items)
	return items
}

func (pq *Queue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.items)
}

// used when a job is finished (Success) or permanently failed.
func (q *Queue) DeleteJob(jobId string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, item := range q.items {
		if item.Job.JobId.Hex() == jobId {
			// Remove element from slice
			q.items = append(q.items[:i], q.items[i+1:]...)
			return true
		}
	}
	return false
}

// when claimed by worker, the job is given on lease to the worker until it is finished successfully
func (q *Queue) Lease(workerId string, lease time.Duration) *QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()

	for i := range q.items { // iterate over all, and return the first find
		item := q.items[i]

		if now.After(item.VisibleAt) {
			item.VisibleAt = now.Add(lease)
			item.ClaimedBy = workerId
			item.ClaimedAt = now
			return item
		}
	}
	return nil
}

// resets the visibility of a job so it can be picked up again.
func (q *Queue) ReleaseLease(jobId string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.items {
		item := q.items[i]
		if item.Job.JobId.Hex() == jobId {
			// Reset visibility to now so another worker can lease it immediately
			item.VisibleAt = time.Now()
			item.ClaimedBy = ""
			return true
		}
	}
	return false
}
