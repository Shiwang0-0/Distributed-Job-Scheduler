package watcher

import (
	"bytes"
	"context"
	"distributed-job-scheduler/pkg/gateway"
	"distributed-job-scheduler/pkg/queue"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func NewWatcher(mongoURI, dbName, port string) (*Watcher, error) {
	ctx, cancle := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancle()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	watcherID := fmt.Sprintf("watcher-%s-%d", port, time.Now().Unix())
	log.Printf("Watcher:%s Connected to MongoDB", watcherID)

	db := client.Database(dbName)

	watcher := &Watcher{
		client:    client,
		db:        db,
		port:      port,
		watcherId: watcherID,
		stats:     &WatcherStats{},
	}

	// add a jitter to prevent Thunder Herd during polling
	baseTimer := 2 * time.Second
	jitter := time.Duration(rand.Intn(2000)) * time.Millisecond
	interval := baseTimer + jitter

	log.Printf("Watcher:%s Started Claiming with %d polling interval", watcherID, interval)

	watcher.JobClaimPoller(interval)

	watcher.RecoverStaleJobs(30 * time.Second) // check for jobs that were failed
	//  because the watcher crashed or died before it could rollback.

	return watcher, nil

}

func (watcher *Watcher) JobClaimPoller(interval time.Duration) {
	watcher.wg.Add(1)
	go func() {
		defer watcher.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				watcher.HandleJobClaim()
			case <-watcher.stopChannel:
				log.Printf("Watcher:%s Job claim poller stopped", watcher.watcherId)
				return
			}
		}
	}()
}

// update the status of job from pending to claimed
func (watcher *Watcher) HandleJobClaim() {
	ctx := context.Background()
	collection := watcher.db.Collection("jobs")

	maxBatch := 5
	claimedCount := 0

	for claimedCount < maxBatch {
		now := time.Now()
		// fmt.Printf("Timer while claiming: %+v\n", now)

		// find jobs that are ready to be claimed
		filter := bson.M{
			"status":       "pending",
			"type":         gateway.JobTypeOnce, // jobs that are only for once will be taken from the DB
			"scheduled_at": bson.M{"$lte": now}, // now or past both
			"$and": []bson.M{
				{
					"$or": []bson.M{
						{"claimed_by": bson.M{"$exists": false}},
						{"claimed_by": nil},
						{"claimed_by": ""},
					},
				},
				// Must not have retry_after, or retry_after has passed
				{
					"$or": []bson.M{
						{"retry_after": bson.M{"$exists": false}},
						{"retry_after": nil},
						{"retry_after": bson.M{"$lte": now}},
					},
				},
			},
		}

		update := bson.M{
			"$set": bson.M{
				"status":     "claimed",
				"claimed_by": watcher.watcherId,
				"claimed_at": now,
				"updated_at": now,
			},
		}

		// set limit that this watcher can add to queue (so task is divided and no watcher gets overloaded)
		// another prevention to Thunder Herd
		// also use FindOneAndUpdate: this is atomic read and write at once
		// in Find and then Update two workers read the same job (and only one write, which makes the other read useless)
		opts := options.FindOneAndUpdate().
			SetSort(bson.D{{Key: "scheduled_at", Value: 1}}).
			SetReturnDocument(options.After)

		var job gateway.Job
		err := collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&job)

		if err != nil {
			if err == mongo.ErrNoDocuments {
				break
			}
			log.Printf("Watcher:%s claim error: %v", watcher.watcherId, err)
			break
		}

		fmt.Printf("Job to be claimed: %+v\n", job)
		watcher.stats.IncrementClaimedJobCount()

		job.ClaimedBy = watcher.watcherId
		job.ClaimedAt = &now
		job.Status = "claimed"

		watcher.addJobToQueue(&job)
		claimedCount++
	}
}

// pushed job in the queue
// updated status from claimed to queued
func (watcher *Watcher) addJobToQueue(job *gateway.Job) {
	ctx := context.Background()
	collection := watcher.db.Collection("jobs")

	now := time.Now()

	payload := queue.QueueItem{
		Job: *job,
	}

	body, _ := json.Marshal(payload)

	resp, err := http.Post(
		"http://localhost:6000/queue/push",
		"application/json",
		bytes.NewReader(body),
	)

	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated) {
		log.Printf("Watcher:%s failed to push job %s", watcher.watcherId, job.JobId)

		// if watcher died before rollback, then RecoverStaleJobs takes care of it

		watcher.rollbackClaimedJobs(job, ctx, collection, now)

		return
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": job.JobId},
		bson.M{"$set": bson.M{
			"status":      "queued",
			"queued_at":   now,
			"updated_at":  now,
			"retry_count": 0,
		},
			"$unset": bson.M{
				"retry_after": "",
			}},
	)

	if err != nil {
		log.Printf("Watcher:%s Error updating job %s to queued: %v", watcher.watcherId, job.JobId, err)
	} else {
		log.Printf("Watcher:%s queued Job %s", watcher.watcherId, job.JobId)
		watcher.stats.IncrementQueuedJobCount()
	}
}

func (watcher *Watcher) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats := watcher.stats.GetStats()
	stats["watcher_id"] = watcher.watcherId
	stats["uptime"] = time.Since(watcher.stats.LastClaimedTime).String()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (watcher *Watcher) rollbackClaimedJobs(job *gateway.Job, ctx context.Context, collection *mongo.Collection, now time.Time) {

	// exponential backoff delay
	retryCount := job.RetryCount
	if retryCount == 0 {
		retryCount = 1
	}

	backoffSeconds := 1 << uint(retryCount) // 2^retrycount capped at 300 seconds
	if backoffSeconds > 300 {
		backoffSeconds = 300
	}
	retryAfter := now.Add(time.Duration(backoffSeconds) * time.Second)

	_, rollbackErr := collection.UpdateOne(
		ctx,
		bson.M{"_id": job.JobId},
		bson.M{
			"$set": bson.M{
				"status":      "pending",
				"claimed_by":  nil,
				"claimed_at":  nil,
				"retry_after": retryAfter,
				"updated_at":  now,
			},
			"$inc": bson.M{
				"retry_count": 1,
			},
		},
	)

	if rollbackErr != nil {
		log.Printf("Watcher:%s CRITICAL: Failed to rollback job %s: %v",
			watcher.watcherId, job.JobId, rollbackErr)
	} else {
		log.Printf("Watcher:%s Rolled back job %s, will retry after %s (attempt %d)",
			watcher.watcherId, job.JobId, retryAfter.Format(time.RFC3339),
			job.RetryCount+1)
	}

}

func (watcher *Watcher) RecoverStaleJobs(interval time.Duration) {
	watcher.wg.Add(1)
	go func() {
		defer watcher.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				watcher.handleStaleJobRecovery()
			case <-watcher.stopChannel:
				return
			}
		}
	}()
}

func (watcher *Watcher) handleStaleJobRecovery() {
	ctx := context.Background()
	collection := watcher.db.Collection("jobs")

	// get jobs claimed more than 30 seconds but not queued
	staleThreshold := time.Now().Add(-30 * time.Second)

	filter := bson.M{
		"status":     "claimed",
		"claimed_at": bson.M{"$lt": staleThreshold},
	}

	update := bson.M{"$set": bson.M{
		"status":     "pending",
		"claimed_by": nil,
		"claimed_at": nil,
		"updated_at": time.Now(),
	}}

	result, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		log.Printf("Watcher:%s Error recovering stale jobs: %v", watcher.watcherId, err)
	} else if result.ModifiedCount > 0 {
		log.Printf("Watcher:%s Recovered %d stale jobs", watcher.watcherId, result.ModifiedCount)
	}
}
