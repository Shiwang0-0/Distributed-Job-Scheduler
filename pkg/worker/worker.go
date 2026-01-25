package worker

import (
	"bytes"
	"context"
	"distributed-job-scheduler/pkg/gateway"
	"distributed-job-scheduler/pkg/queue"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func NewWorker(mongoURI, dbName, port, coordinatorPort string, concurrency int) (*Worker, error) {
	ctx, cancle := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancle()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	workerId := fmt.Sprintf("worker-%s-%d", port, time.Now().Unix())
	log.Printf("Worker:%s Connected to MongoDB", workerId)

	db := client.Database(dbName)

	worker := &Worker{
		client:          client,
		db:              db,
		workerId:        workerId,
		port:            port,
		coordinatorPort: coordinatorPort,
		stats:           &WorkerStats{},
		stopChannel:     make(chan struct{}),
		concurrency:     concurrency,
		semaphore:       make(chan struct{}, concurrency),
	}

	if err := worker.registerWithCoordinator(); err != nil {
		return nil, fmt.Errorf("failed to register with coordinator: %w", err)
	}
	// send heart beats to coordinator
	worker.startHeartbeat()

	worker.startJobPuller()

	log.Printf("Worker:%s Started ", workerId)

	return worker, nil
}

func (worker *Worker) registerWithCoordinator() error {
	reqBody := map[string]string{"worker_id": worker.workerId}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("http://localhost:%s/coordinator/request-assignment", worker.coordinatorPort)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to connect to coordinator: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coordinator returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var assignment struct {
		WorkerID  string `json:"worker_id"`
		QueuePort string `json:"queue_port"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&assignment); err != nil {
		return fmt.Errorf("failed to decode assignment: %w", err)
	}

	worker.assignedQueuePort = assignment.QueuePort
	log.Printf("Worker:%s Assigned to queue port %s by coordinator", worker.workerId, worker.assignedQueuePort)

	return nil
}

func (worker *Worker) startHeartbeat() {
	worker.wg.Add(1)
	go func() {
		defer worker.wg.Done()
		ticker := time.NewTicker(10 * time.Second) // every 10s
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				worker.sendHeartbeat()
			case <-worker.stopChannel:
				return
			}
		}
	}()
}

func (worker *Worker) sendHeartbeat() {
	reqBody := map[string]string{"worker_id": worker.workerId}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("http://localhost:%s/coordinator/heartbeat", worker.coordinatorPort)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("Worker:%s Failed to send heartbeat: %v", worker.workerId, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Worker:%s Heartbeat failed with status %d", worker.workerId, resp.StatusCode)
	}
}

func (worker *Worker) startJobPuller() {
	worker.wg.Add(1)
	go func() {
		defer worker.wg.Done()
		baseInterval := 2 * time.Second
		log.Printf("Worker:%s Job puller started with %d concurrency", worker.workerId, worker.concurrency)

		for {
			jitter := time.Duration(rand.Int63n(int64(baseInterval / 5)))
			select {
			case <-time.After(baseInterval + jitter):

				select {
				case worker.semaphore <- struct{}{}:
					// Slot acquired (running fewer than 5 jobs), proceed to pull
				default:
					// All slots full, skip this tick and wait for next interval
					continue
				}

				job, err := worker.pullJob()

				if err != nil || job == nil {
					<-worker.semaphore // release if no job was found
					// log.Printf("Worker:%s Failed to pull job: %v", worker.workerId, err)
					continue
				}

				worker.wg.Add(1)
				go func(job *gateway.Job) {
					defer worker.wg.Done()
					defer func() { <-worker.semaphore }()

					worker.stats.IncrementRunningCount()
					defer worker.stats.DecrementRunningCount()

					worker.executeJob(job)
				}(job)

			case <-worker.stopChannel:
				log.Printf("Worker:%s Job puller stopped", worker.workerId)
				return
			}

		}
	}()
}

func (worker *Worker) pullJob() (*gateway.Job, error) {
	reqBody := map[string]string{
		"worker_id": worker.workerId,
	}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("http://localhost:%s/queue/lease", worker.assignedQueuePort)
	res, err := http.Post(url, "application/json", bytes.NewReader(body))

	if err != nil {
		log.Printf("Worker:%s Error pulling job: %v", worker.workerId, err)
		return nil, fmt.Errorf("failed to Connect to queue: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode == http.StatusNoContent {
		return nil, fmt.Errorf("queue empty")
	}

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("queue returned status %d: %s", res.StatusCode, string(body))
	}

	var queueItem queue.QueueItem
	if err := json.NewDecoder(res.Body).Decode(&queueItem); err != nil {
		return nil, fmt.Errorf("failed to decode job: %w", err)
	}

	log.Printf("Worker:%s Pulled job %s from Queue",
		worker.workerId, queueItem.Job.JobId)

	return &queueItem.Job, nil
}

func (worker *Worker) executeJob(job *gateway.Job) {
	startTime := time.Now()

	executionId := worker.prepareToExecute(job, startTime)

	_, execErr := worker.performJobWork(job)
	finishedAt := time.Now()

	status := "success"
	finalJobStatus := "completed"
	errorMsg := ""

	if execErr != nil {
		status = "failed"
		worker.stats.IncrementFailureCount()
		errorMsg = execErr.Error()

		if job.RetryCount < job.MaxRetries {
			job.RetryCount++
			finalJobStatus = "pending"

			log.Printf("Worker:%s Job %s failed, retry %d/%d",
				worker.workerId, job.JobId.Hex(), job.RetryCount, job.MaxRetries)
		} else {
			finalJobStatus = "failed"

			log.Printf("Worker:%s Job %s failed permanently after %d retries",
				worker.workerId, job.JobId.Hex(), job.MaxRetries)
			log.Printf("CRITICAL: Job %s exhausted all retries...", job.JobId.Hex())
		}
	}

	worker.db.Collection("job_executions").UpdateOne(
		context.Background(),
		bson.M{"_id": executionId},
		bson.M{"$set": bson.M{
			"finished_at":   finishedAt,
			"status":        status,
			"error_message": errorMsg,
		}},
	)

	switch finalJobStatus {
	case "pending":
		// Job needs retry - update in jobs collection
		worker.finalizeJobInDB(job, finalJobStatus, finishedAt)
		worker.releaseLease(job.JobId.Hex())
	case "completed":
		// Job succeeded - move to completed_jobs
		worker.moveToCompletedJobs(job, finishedAt)
		worker.stats.IncrementSuccessCount()
		log.Printf("SUCCESS: Job %s Executed...", job.JobId.Hex())
	case "failed":
		// Job permanently failed - move to failed_jobs
		worker.moveToFailedJobs(job, finishedAt, errorMsg)
		worker.stats.IncrementFailureCount()
	}

	if finalJobStatus == "completed" || finalJobStatus == "failed" {
		worker.deleteFromQueue(job.JobId.Hex())
	}
}

func (worker *Worker) deleteFromQueue(jobId string) {
	url := fmt.Sprintf("http://localhost:%s/queue/delete?job_id=%s", worker.assignedQueuePort, jobId)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		log.Printf("Worker:%s Failed to create delete request: %v", worker.workerId, err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Worker:%s Failed to send delete to Watcher: %v", worker.workerId, err)
		return
	}
	defer resp.Body.Close()
}

func (worker *Worker) releaseLease(jobId string) {
	// We call a release lease endpoint that resets the 'VisibleAt' timer
	url := fmt.Sprintf("http://localhost:%s/queue/release-lease?job_id=%s", worker.assignedQueuePort, jobId)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		log.Printf("Worker:%s Failed to send NACK to Watcher: %v", worker.workerId, err)
		return
	}
	log.Printf("Worker:%s Released Lease", worker.workerId)
	defer resp.Body.Close()
}
