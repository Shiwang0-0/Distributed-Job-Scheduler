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
			Keys: bson.D{{Key: "next_run_at", Value: 1}},
		},
	}

	_, err := jobsCollection.Indexes().CreateMany(ctx, jobIndexes)
	if err != nil {
		return err
	}

	// execution --> store the jobs that have started executing
	executionsCollection := db.Collection("executions")
	executionsIndexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "started_at", Value: -1}},
		},
	}

	_, err = executionsCollection.Indexes().CreateMany(ctx, executionsIndexes)
	if err != nil {
		return err
	}
	return nil
}
