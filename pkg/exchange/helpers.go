package exchange

import (
	"crypto/sha256"
	"distributed-job-scheduler/pkg/gateway"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

/*
	Other than mentioned High Priority Job
	Treat CRON Jobs, Retrying Jobs as High priority Jobs too

	All Others are Treated as Normal Jobs

	The idea is to give more high performing workers to High Priority Queues

*/

func (e *Exchange) determineJobPriority(job *gateway.Job) gateway.JobPriority {
	if job.Priority == gateway.PriorityHigh {
		return gateway.PriorityHigh
	}

	if job.Type == gateway.JobTypeCron {
		log.Printf("Exchange:%s Making CRON job %s to HIGH priority", e.exchangeID, job.JobId.Hex())
		return gateway.PriorityHigh
	}

	if job.RetryCount > 0 {
		log.Printf("Exchange:%s Making retry job %s to HIGH priority", e.exchangeID, job.JobId.Hex())
		return gateway.PriorityHigh
	}

	if job.Priority != "" {
		return job.Priority
	}

	return gateway.PriorityNormal
}

func (e *Exchange) hashToQueueIndex(job *gateway.Job, numQueues int) int {
	// retry_count in hash - different retry = different queue
	hashInput := fmt.Sprintf("%s:retry%d", job.Type, job.RetryCount)

	// SHA256 hash
	hash := sha256.Sum256([]byte(hashInput))

	// first 8 bytes to uint64
	hashValue := binary.BigEndian.Uint64(hash[:8])

	// get queue index (fixed N)
	queueIndex := int(hashValue % uint64(numQueues))

	return queueIndex
}

func (e *Exchange) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats := e.stats.GetStats()
	stats["exchange_id"] = e.exchangeID
	stats["port"] = e.port
	stats["normal_queues"] = e.normalQueuePorts
	stats["normal_queues_count"] = len(e.normalQueuePorts)
	stats["high_priority_queues"] = e.highQueuePorts
	stats["high_priority_queues_count"] = len(e.highQueuePorts)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (e *Exchange) Shutdown() {
	close(e.stopChan)
	e.wg.Wait()
	log.Printf("Exchange:%s Shutdown complete", e.exchangeID)
}
