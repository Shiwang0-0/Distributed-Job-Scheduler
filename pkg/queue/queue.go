package queue

func NewQueue() *Queue {
	return &Queue{
		items: make([]QueueItem, 0),
	}
}

func (q *Queue) Push(item QueueItem) {
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

	return &item
}

func (q *Queue) Peek() *QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.items) == 0 {
		return nil
	}

	return &q.items[0]
}

func (q *Queue) GetAll() []QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	items := make([]QueueItem, len(q.items))
	copy(items, q.items)
	return items
}

func (pq *Queue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.items)
}
