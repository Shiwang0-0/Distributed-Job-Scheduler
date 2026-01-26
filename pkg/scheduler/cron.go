package scheduler

import (
	"context"
	"distributed-job-scheduler/pkg/cron"
	"distributed-job-scheduler/pkg/gateway"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (s *Scheduler) CronJobLoader(ctx context.Context) error {
	filter := bson.M{
		"type":   gateway.JobTypeCron,
		"status": "active",
	}

	cursor, err := s.db.Collection("jobs").Find(ctx, filter)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	count := 0
	for cursor.Next(ctx) {
		var job gateway.Job
		if err := cursor.Decode(&job); err != nil {
			log.Printf("Scheduler:%s Failed to decode cron job: %v", s.port, err)
			continue
		}

		if job.CronExpr == "" {
			log.Printf("Scheduler:%s Cron job %s has empty expression", s.port, job.JobId.Hex())
			continue
		}

		count++
		log.Printf("Scheduler:%s Loaded cron job %s [%s]", s.port, job.JobId.Hex(), job.CronExpr)
	}

	log.Printf("Scheduler:%s Loaded %d cron jobs", s.port, count)
	return cursor.Err()
}

// runs every minute to check for cron jobs that need execution
func (s *Scheduler) CronTicker() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Printf("Scheduler:%s Cron ticker started (LEADER)", s.port)

	for {
		select {
		case <-ticker.C:
			// Only process if still leader
			if s.IsLeader() {
				s.checkCronJobs()
			} else {
				log.Printf("Scheduler:%s Cron ticker stopped (no longer leader)", s.port)
				return
			}
		case <-s.stopChan:
			log.Printf("Scheduler:%s Cron ticker stopped", s.port)
			return
		}
	}
}

func (s *Scheduler) checkCronJobs() {
	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)

	filter := bson.M{
		"type":   gateway.JobTypeCron,
		"status": "active",
		"$or": []bson.M{
			{"next_run_at": bson.M{"$lte": now}},
			{"next_run_at": bson.M{"$exists": false}},
		},
	}

	cursor, err := s.db.Collection("jobs").Find(ctx, filter)
	if err != nil {
		log.Printf("Scheduler:%s Failed to query cron jobs: %v", s.port, err)
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var cronJob gateway.Job
		if err := cursor.Decode(&cronJob); err != nil {
			log.Printf("Scheduler:%s Failed to decode cron job: %v", s.port, err)
			continue
		}

		// Execute and update atomically
		s.executeCronJob(ctx, cronJob)
	}

	if err := cursor.Err(); err != nil {
		log.Printf("Scheduler:%s Cursor error while checking cron jobs: %v", s.port, err)
	}
}

// executeCronJob creates a new job instance for the cron job
func (s *Scheduler) executeCronJob(ctx context.Context, cronJob gateway.Job) {
	now := time.Now()

	if cronJob.CronExpr == "" {
		log.Printf("Scheduler:%s Cron job %s has empty cron expression", s.port, cronJob.JobId.Hex())
		return
	}

	schedule, err := cron.ParseCronExpr(cronJob.CronExpr)
	if err != nil {
		log.Printf("Scheduler:%s Failed to parse cron expression for job %s (%s): %v",
			s.port, cronJob.JobId.Hex(), cronJob.CronExpr, err)
		return
	}

	// Calculate next run time
	nextRun := schedule.Next(now)

	// First, atomically update the parent cron job to prevent duplicate execution
	update := bson.M{
		"$set": bson.M{
			"last_run_at": now,
			"next_run_at": nextRun,
			"updated_at":  now,
		},
	}

	// Optimistic locking: only update if next_run_at hasn't changed
	filter := bson.M{
		"_id":  cronJob.JobId,
		"type": gateway.JobTypeCron,
	}

	// Only lock on next_run_at if it's set (not first run)
	if !cronJob.NextRunAt.IsZero() {
		filter["next_run_at"] = cronJob.NextRunAt
	}

	result := s.db.Collection("jobs").FindOneAndUpdate(
		ctx,
		filter,
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)

	if result.Err() != nil {
		if result.Err() == mongo.ErrNoDocuments {
			// Another instance already updated this job
			log.Printf("Scheduler:%s Cron job %s already processed by another instance", s.port, cronJob.JobId.Hex())
			return
		}
		log.Printf("Scheduler:%s Failed to update cron job %s: %v", s.port, cronJob.JobId.Hex(), result.Err())
		return
	}

	// Now create a new "once" job instance
	// This will be picked up by the Watcher -> Queue -> Worker flow
	newJob := gateway.Job{
		Type:        gateway.JobTypeOnce,
		Payload:     cronJob.Payload,
		MaxRetries:  cronJob.MaxRetries,
		CreatedAt:   now,
		UpdatedAt:   now,
		ScheduledAt: now,
		Status:      "pending", // Watcher will claim this
		RetryCount:  0,
	}

	insertResult, err := s.db.Collection("jobs").InsertOne(ctx, newJob)
	if err != nil {
		log.Printf("Scheduler:%s Failed to create job instance for cron job %s: %v", s.port, cronJob.JobId.Hex(), err)
		return
	}

	log.Printf("Scheduler:%s Created job instance %s from cron job %s (next run: %s)",
		s.port,
		insertResult.InsertedID.(primitive.ObjectID).Hex(),
		cronJob.JobId.Hex(),
		nextRun.Format(time.RFC3339))
}
