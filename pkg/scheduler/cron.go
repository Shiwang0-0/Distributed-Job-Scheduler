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

		schedule, err := cron.ParseCronExpr(job.CronExpr)
		if err != nil {
			log.Printf("Scheduler:%s Failed to parse cron job %s: %v", s.port, job.JobId.Hex(), err)
			continue
		}

		s.cronMutex.Lock()
		s.cronJobs[job.JobId.Hex()] = schedule
		s.cronMutex.Unlock()

		count++
		log.Printf("Scheduler:%s Loaded cron job %s [%s]", s.port, job.JobId.Hex(), job.CronExpr)
	}

	log.Printf("Scheduler:%s Loaded %d cron jobs", s.port, count)
	return cursor.Err()
}

// runs every minute to check for cron jobs that need execution
func (s *Scheduler) CronTicker() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
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

	s.cronMutex.RLock()
	jobIDs := make([]string, 0, len(s.cronJobs))
	for jobID := range s.cronJobs {
		jobIDs = append(jobIDs, jobID)
	}
	s.cronMutex.RUnlock()

	for _, jobIDStr := range jobIDs {
		jobID, err := primitive.ObjectIDFromHex(jobIDStr)
		if err != nil {
			continue
		}

		// Use FindOneAndUpdate to JobTypeCron atomically check and update next_run_at
		// This prevents multiple scheduler instances from creating duplicate jobs
		filter := bson.M{
			"_id":    jobID,
			"type":   gateway.JobTypeCron,
			"status": "active",
			"$or": []bson.M{
				{"next_run_at": bson.M{"$lte": now}},
				{"next_run_at": bson.M{"$exists": false}},
			},
		}

		var cronJob gateway.Job
		err = s.db.Collection("jobs").FindOne(ctx, filter).Decode(&cronJob)

		if err != nil {
			if err == mongo.ErrNoDocuments {
				// Job was deleted or already processed
				continue
			}
			log.Printf("Scheduler:%s Failed to fetch cron job %s: %v", s.port, jobIDStr, err)
			continue
		}

		// Execute and update atomically
		s.executeCronJob(ctx, cronJob)
	}
}

// executeCronJob creates a new job instance for the cron job
func (s *Scheduler) executeCronJob(ctx context.Context, cronJob gateway.Job) {
	now := time.Now()

	// Get schedule from memory
	s.cronMutex.RLock()
	schedule, exists := s.cronJobs[cronJob.JobId.Hex()]
	s.cronMutex.RUnlock()

	if !exists {
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

	result := s.db.Collection("jobs").FindOneAndUpdate(
		ctx,
		bson.M{
			"_id":         cronJob.JobId,
			"type":        gateway.JobTypeCron,
			"next_run_at": cronJob.NextRunAt, // Optimistic locking
		},
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
		QueuedAt:    now,
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
