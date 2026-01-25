package exchange

import (
	"bytes"
	"distributed-job-scheduler/pkg/gateway"
	"distributed-job-scheduler/pkg/queue"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func NewExchange(port string, normalQueuePorts, highQueuePorts []string) *Exchange {
	exchangeID := fmt.Sprintf("exchange-%s", port)

	log.Printf("Exchange:%s Starting on port %s", exchangeID, port)

	return &Exchange{
		port:                port,
		exchangeID:          exchangeID,
		normalQueuePorts:    normalQueuePorts,
		highQueuePorts:      highQueuePorts,
		healthyNormalQueues: normalQueuePorts,
		healthyHighQueues:   highQueuePorts,
		stats:               &ExchangeStats{},
		stopChan:            make(chan struct{}),
	}
}

func (e *Exchange) HandleRouteJob(w http.ResponseWriter, r *http.Request) {
	var item queue.QueueItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		e.stats.IncrementErrors()
		return
	}

	targetQueuePort, err := e.routeJob(&item.Job)
	if err != nil {
		http.Error(w, fmt.Sprintf("Routing error: %v", err), http.StatusServiceUnavailable)
		e.stats.IncrementErrors()
		return
	}

	if err := e.forwardToQueue(targetQueuePort, &item); err != nil {
		// If forwarding fails, try to reroute to another healthy queue
		log.Printf("Exchange:%s Failed to forward to queue %s, attempting reroute: %v",
			e.exchangeID, targetQueuePort, err)

		http.Error(w, fmt.Sprintf("Failed to forward to queue: %v", err), http.StatusInternalServerError)
		e.stats.IncrementErrors()
		return
	}

	e.stats.IncrementRouted()
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "routed",
		"queue":  targetQueuePort,
	})
}

func (e *Exchange) routeJob(job *gateway.Job) (string, error) {
	priority := e.determineJobPriority(job)

	var queues []string
	if priority == gateway.PriorityHigh {
		queues = e.highQueuePorts
	} else {
		queues = e.normalQueuePorts
	}

	/* 	to determine which queue to send the job to
	Hash with retry_count for redistribution on failures
	retry_count=0: tries one queue
	retry_count=1: tries different queue
	retry_count=2: tries yet another queue
	*/

	queueIndex := e.hashToQueueIndex(job, len(queues))
	targetQueuePort := queues[queueIndex]

	if priority == gateway.PriorityHigh {
		log.Printf("Exchange:%s Routing HIGH priority job %s type:%s to queue %s [%d/%d healthy queues]",
			e.exchangeID, job.JobId.Hex(), job.Type, targetQueuePort, len(queues), len(e.highQueuePorts))
	} else {
		log.Printf("Exchange:%s Routing NORMAL priority job %s type:%s to queue %s [%d/%d healthy queues]",
			e.exchangeID, job.JobId.Hex(), job.Type, targetQueuePort, len(queues), len(e.normalQueuePorts))
	}

	return targetQueuePort, nil
}

func (e *Exchange) forwardToQueue(queuePort string, item *queue.QueueItem) error {
	body, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%s/queue/push", queuePort)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to push to queue %s: %w", queuePort, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("queue %s returned status %d", queuePort, resp.StatusCode)
	}

	return nil
}
