package queue

import (
	"log"
)

func NewQueue() *Queue {
	return &Queue{
		Items: make([]QueueItem, 0),
	}
}

func (q *Queue) Push(item QueueItem) {
	q.Mu.Lock()
	defer q.Mu.Unlock()

	q.Items = append(q.Items, item)

	log.Printf("Added job %+v to Queue, current queue size: %d)",
		item.Job.JobId, len(q.Items))
}

func (q *Queue) Pop() *QueueItem {
	q.Mu.Lock()
	defer q.Mu.Unlock()

	if len(q.Items) == 0 {
		return nil
	}
	item := q.Items[0]
	q.Items = q.Items[1:]

	log.Printf("Popped job %d from Queue, current queue size: %d)",
		item.Job.JobId, len(q.Items))

	return &item
}

func (q *Queue) Peek() *QueueItem {
	q.Mu.RLock()
	defer q.Mu.RUnlock()

	if len(q.Items) == 0 {
		return nil
	}

	return &q.Items[0]
}

func (pq *Queue) Size() int {
	pq.Mu.RLock()
	defer pq.Mu.RUnlock()
	return len(pq.Items)
}
