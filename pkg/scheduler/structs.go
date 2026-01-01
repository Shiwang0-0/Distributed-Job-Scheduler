package scheduler

import "go.mongodb.org/mongo-driver/mongo"

type Scheduler struct {
	client     *mongo.Client
	db         *mongo.Database
	port       string
	instanceId string
}
