package worker

import (
	"bytes"
	"context"
	"distributed-job-scheduler/pkg/gateway"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (worker *Worker) performJobWork(job *gateway.Job) (map[string]interface{}, error) {

	payloadMap := job.Payload

	jobType, ok := payloadMap["job_type"].(string)
	if !ok {
		return nil, fmt.Errorf("'job_type' field is required in payload")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Execute using job registry
	log.Printf("Worker:%s Executing job type: %s", worker.workerId, jobType)

	result, err := worker.jobRegistry.Execute(ctx, jobType, payloadMap)
	if err != nil {
		return result, err
	}

	// Add worker metadata to result
	if result == nil {
		result = make(map[string]interface{})
	}

	result["payload"] = job.Payload
	result["worker_id"] = worker.workerId
	result["executed_at"] = time.Now()
	return result, nil
}

func getJobType(payload interface{}) string {
	if payloadMap, ok := payload.(map[string]interface{}); ok {
		if jobType, ok := payloadMap["job_type"].(string); ok {
			return jobType
		}
	}
	return "unknown"
}

func (worker *Worker) moveToCompletedJobs(job *gateway.Job, finishedAt time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create completed job record
	completedJob := bson.M{
		"_id":          job.JobId,
		"type":         job.Type,
		"payload":      job.Payload,
		"retry_count":  job.RetryCount,
		"max_retries":  job.MaxRetries,
		"worker_id":    worker.workerId,
		"created_at":   job.CreatedAt,
		"completed_at": finishedAt,
		"status":       "completed",
	}

	// completed_jobs collection
	_, err := worker.db.Collection("completed_jobs").InsertOne(ctx, completedJob)
	if err != nil {
		log.Printf("Worker:%s Error moving job %s to completed_jobs: %v",
			worker.workerId, job.JobId.Hex(), err)
		return
	}

	// Delete from active jobs collection
	_, err = worker.db.Collection("jobs").DeleteOne(ctx, bson.M{"_id": job.JobId})
	if err != nil {
		log.Printf("Worker:%s Error deleting job %s from jobs: %v",
			worker.workerId, job.JobId.Hex(), err)
	}

	log.Printf("Worker:%s Moved job %s to completed_jobs", worker.workerId, job.JobId.Hex())
}

func (worker *Worker) moveToFailedJobs(job *gateway.Job, finishedAt time.Time, errorMsg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// failed job record
	failedJob := bson.M{
		"_id":           job.JobId,
		"type":          job.Type,
		"payload":       job.Payload,
		"retry_count":   job.RetryCount,
		"max_retries":   job.MaxRetries,
		"worker_id":     worker.workerId,
		"created_at":    job.CreatedAt,
		"failed_at":     finishedAt,
		"status":        "failed",
		"error_message": errorMsg,
	}

	// failed_jobs collection
	_, err := worker.db.Collection("failed_jobs").InsertOne(ctx, failedJob)
	if err != nil {
		log.Printf("Worker:%s Error moving job %s to failed_jobs: %v",
			worker.workerId, job.JobId.Hex(), err)
		return
	}

	// Delete from active jobs collection
	_, err = worker.db.Collection("jobs").DeleteOne(ctx, bson.M{"_id": job.JobId})
	if err != nil {
		log.Printf("Worker:%s Error deleting job %s from jobs: %v",
			worker.workerId, job.JobId.Hex(), err)
	}

	log.Printf("Worker:%s Moved job %s to failed_jobs", worker.workerId, job.JobId.Hex())
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

func (worker *Worker) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats := worker.stats.GetStats()
	stats["worker_id"] = worker.workerId
	stats["concurrency"] = worker.concurrency

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (worker *Worker) Shutdown() {
	// Release assignment from coordinator
	reqBody := map[string]string{"worker_id": worker.workerId}
	body, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("http://localhost:%s/coordinator/release-assignment", worker.coordinatorPort)
	http.Post(url, "application/json", bytes.NewReader(body))

	close(worker.stopChannel)
	worker.wg.Wait()
	worker.client.Disconnect(context.Background())
	log.Printf("Worker:%s Shutdown complete", worker.workerId)
}
