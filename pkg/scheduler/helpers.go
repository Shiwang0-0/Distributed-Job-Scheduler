package scheduler

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func dbIndexes(db *mongo.Database, ctx context.Context) error {
	// we need two collections
	// jobs --> store all kinds of jobs

	jobsCollection := db.Collection("jobs")
	jobIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "scheduled_at", Value: 1}}, // changed from next_run_at
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "scheduled_at", Value: 1},
			},
		},
	}

	_, err := jobsCollection.Indexes().CreateMany(ctx, jobIndexes)
	if err != nil {
		return err
	}

	// execution --> store the jobs that have started executing
	executionsCollection := db.Collection("job_executions")
	executionsIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "started_at", Value: -1}},
		},
	}

	_, err = executionsCollection.Indexes().CreateMany(ctx, executionsIndexes)
	if err != nil {
		return err
	}

	completedJobsCollection := db.Collection("completed_jobs")
	completedJobsIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "completed_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
		},
	}

	_, err = completedJobsCollection.Indexes().CreateMany(ctx, completedJobsIndexes)
	if err != nil {
		return err
	}

	failedJobsCollection := db.Collection("failed_jobs")
	failedJobsIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "failed_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}},
		},
	}

	_, err = failedJobsCollection.Indexes().CreateMany(ctx, failedJobsIndexes)
	if err != nil {
		return err
	}

	return nil
}
