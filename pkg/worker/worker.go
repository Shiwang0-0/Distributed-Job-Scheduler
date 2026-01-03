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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func NewWorker(mongoURI, dbName, port string, concurrency int) (*Worker, error) {
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
		client:      client,
		db:          db,
		workerId:    workerId,
		port:        port,
		stats:       &WorkerStats{},
		stopChannel: make(chan struct{}),
		concurrency: concurrency,
		semaphore:   make(chan struct{}, concurrency),
	}

	worker.startJobPuller()

	log.Printf("Worker:%s Started ", workerId)

	return worker, nil
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
	reqBody := fmt.Sprintf(`"worker_id":"%s"`, worker.workerId)

	res, err := http.Post("http://localhost:6000/queue/lease", "application/json", bytes.NewBufferString(reqBody))

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

	worker.finalizeJobInDB(job, finalJobStatus, finishedAt)

	if execErr == nil || finalJobStatus == "failed" {
		if execErr == nil {
			worker.stats.IncrementSuccessCount()
			log.Printf("SUCCESS: Job %s Executed...", job.JobId.Hex())
		} else {
			worker.stats.IncrementFailureCount()
		}
		worker.deleteFromQueue(job.JobId.Hex())
	} else {
		worker.releaseLease(job.JobId.Hex())
	}

}

func (worker *Worker) prepareToExecute(job *gateway.Job, startTime time.Time) primitive.ObjectID {
	log.Printf("Worker:%s Preparing to Execute: %s ",
		worker.workerId, job.JobId)

	ctx := context.Background()

	execution := gateway.JobExecution{
		JobId:     job.JobId,
		WorkerId:  worker.workerId,
		StartedAt: startTime,
		Status:    "running",
	}

	executionsCollection := worker.db.Collection("job_executions")

	execResult, err := executionsCollection.InsertOne(ctx, execution)
	if err != nil {
		log.Printf("Worker:%s Error Preparing execution: %v", worker.workerId, err)
	}

	var executionId primitive.ObjectID
	if execResult != nil {
		executionId = execResult.InsertedID.(primitive.ObjectID)
	}
	return executionId
}

func (worker *Worker) finalizeJobInDB(job *gateway.Job, finalStatus string, finishedAt time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	update := bson.M{
		"status":      finalStatus,
		"retry_count": job.RetryCount,
		"updated_at":  finishedAt,
		"last_run_at": finishedAt,
	}
	// If the job needs to be retried, we release it from the lease
	if finalStatus == "pending" {
		update["claimed_by"] = nil
		update["claimed_at"] = nil
		update["locked_until"] = nil

		// EXPONENTIAL BACKOFF:
		// Wait (RetryCount^2 * 10) seconds before it can be picked up again
		// retry 1: 10s | retry 2: 40s | retry 3: 90s
		delay := time.Duration(job.RetryCount*job.RetryCount*10) * time.Second
		update["scheduled_at"] = time.Now().Add(delay)

		/* when you release the job, there might be possibility that same worker picks it again ?
		but this possibility is when the worker was down (thats why it couldn't complete the task)
		and if it is down it will either recover in (retry^2 * 10) seconds, if not other workers are free to pick the job
		*/

		log.Printf("Worker:%s Releasing job %s for retry in %v",
			worker.workerId, job.JobId.Hex(), delay)
	}

	_, err := worker.db.Collection("jobs").UpdateOne(
		ctx,
		bson.M{"_id": job.JobId},
		bson.M{"$set": update},
	)

	if err != nil {
		log.Printf("Worker:%s Error finalizing job %s in DB: %v",
			worker.workerId, job.JobId.Hex(), err)
	}
}

func (worker *Worker) deleteFromQueue(jobId string) {
	url := fmt.Sprintf("http://localhost:6000/queue/delete?job_id=%s", jobId)

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
	// We call a relase lease endpoint that resets the 'VisibleAt' timer
	url := fmt.Sprintf("http://localhost:6000/queue/release-lease?job_id=%s", jobId)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		log.Printf("Worker:%s Failed to send NACK to Watcher: %v", worker.workerId, err)
		return
	}
	log.Printf("Worker:%s Released Lease", worker.workerId)
	defer resp.Body.Close()
}

func (worker *Worker) performJobWork(job *gateway.Job) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	result["payload"] = job.Payload
	result["worker_id"] = worker.workerId
	result["executed_at"] = time.Now()
	return result, nil
}

func (worker *Worker) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats := worker.stats.GetStats()
	stats["worker_id"] = worker.workerId
	stats["concurrency"] = worker.concurrency

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
