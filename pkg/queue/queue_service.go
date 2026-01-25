package queue

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func NewQueueService(queueID, port string) *QueueService {
	return &QueueService{
		queue:   NewQueue(),
		queueID: queueID,
		port:    port,
		stats:   &QueueStats{},
	}
}

func (qs *QueueService) HandlePush(w http.ResponseWriter, r *http.Request) {
	var item *QueueItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	qs.queue.Push(item)
	qs.stats.IncrementPushed()

	log.Printf("Added job %s, in queue %s size=%d", item.Job.JobId.Hex(), qs.queueID, qs.queue.Size())

	w.WriteHeader(http.StatusCreated)
}

/*
func (qs *QueueService) HandlePop(w http.ResponseWriter, r *http.Request) {
	item := qs.queue.Pop()
	if item == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	qs.stats.IncrementPopped()

	json.NewEncoder(w).Encode(item)

	log.Printf("Popped job %s, queue size=%d",
		item.Job.JobId.Hex(),
		qs.queue.Size(),
	)
}
*/

func (qs *QueueService) HandleDelete(w http.ResponseWriter, r *http.Request) {
	jobId := r.URL.Query().Get("job_id")
	if jobId == "" {
		http.Error(w, "missing job_id", http.StatusBadRequest)
		return
	}

	found := qs.queue.DeleteJob(jobId)
	if !found {
		http.Error(w, "job not found in local queue", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (qs *QueueService) HandleLease(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkerId string `json:"worker_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	leaseDuration := 30 * time.Second

	item := qs.queue.Lease(body.WorkerId, leaseDuration)
	if item == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	qs.stats.IncrementPopped()

	json.NewEncoder(w).Encode(item)

	log.Printf("Queue %s Leased job %s to %s until %v", qs.queueID, item.Job.JobId.Hex(), body.WorkerId, item.VisibleAt)
}

func (qs *QueueService) HandleReleaseLease(w http.ResponseWriter, r *http.Request) {
	jobId := r.URL.Query().Get("job_id")
	if jobId == "" {
		http.Error(w, "missing job_id", http.StatusBadRequest)
		return
	}

	found := qs.queue.ReleaseLease(jobId)
	if !found {
		http.Error(w, "job not found in local queue", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (qs *QueueService) HandlePeek(w http.ResponseWriter, r *http.Request) {
	log.Printf("Peeked job")
	item := qs.queue.Peek()
	if item == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	json.NewEncoder(w).Encode(item)

	log.Printf("Queue %s Peeked job %s", qs.queueID, item.Job.JobId.Hex())
}

func (qs *QueueService) HandleGetAll(w http.ResponseWriter, r *http.Request) {
	items := qs.queue.GetAll()
	json.NewEncoder(w).Encode(items)

	log.Printf("Queue:%s Returned %d jobs", qs.queueID, len(items))
}

func (qs *QueueService) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats := qs.stats.Get()
	json.NewEncoder(w).Encode(stats)
}
