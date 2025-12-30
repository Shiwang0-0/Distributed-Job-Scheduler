package scheduler

import (
	"context"
	"distributed-job-scheduler/pkg/gateway"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Scheduler struct {
	client     *mongo.Client
	db         *mongo.Database
	port       string
	instanceId string
}

func NewScheduler(mongoURI, dbName, port string) (*Scheduler, error) {
	ctx, cancle := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancle()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	log.Printf("Scheduler:%s Connected to MongoDB successfully", port)

	db := client.Database(dbName)

	err = dbIndexes(db, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create indexes for the collections: %w", err)
	}
	log.Printf("Scheduler:%s Database indexes created", port)

	scheduler := &Scheduler{
		client:     client,
		db:         db,
		port:       port,
		instanceId: fmt.Sprintf("scheduler-instance-%s", port),
	}
	log.Printf("Scheduler:%s Job scheduler started", port)

	return scheduler, nil
}

func (s *Scheduler) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// writes the job to the database
func (s *Scheduler) HandleSchedule(w http.ResponseWriter, r *http.Request) {
	log.Println("Job received")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var job gateway.Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		log.Printf("Scheduler:%s Invalid request body: %v", s.port, err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	fmt.Printf("curr job: %+v", job.Payload)

	// Set default values
	now := time.Now()

	if job.ScheduledAt.IsZero() {
		job.ScheduledAt = now
	}

	if job.MaxRetries == 0 {
		job.MaxRetries = 5 // Default max retries
	}

	job.Status = "PENDING"
	job.RetryCount = 0
	job.CreatedAt = now

	log.Printf("Scheduler:%s scheduled: %s",
		s.port, job.ScheduledAt.Format(time.RFC3339))

	ctx, cancle := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancle()

	collection := s.db.Collection("jobs")
	result, err := collection.InsertOne(ctx, job)
	if err != nil {
		log.Printf("Scheduler:%s Error writing job to DB: %v", s.port, err)
		http.Error(w, "Failed to create job in database", http.StatusInternalServerError)
		return
	}

	job.JobId = result.InsertedID.(primitive.ObjectID)

	log.Printf("Scheduler:%s wrote to DB: %s (status: pending)", s.port, job.JobId)

	res := gateway.JobResponse{
		JobId:   job.JobId.Hex(),
		Status:  "pending",
		Message: fmt.Sprintf("Job scheduled successfully on %s. Watcher will process it.", s.instanceId),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(res)
}

func (s *Scheduler) HandleGetJobs(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	collection := s.db.Collection("jobs")

	// search for specific status job
	// ?status="pending"
	status := r.URL.Query().Get("status")

	filter := bson.M{}
	if status != "" {
		filter["status"] = status
	}

	// find returns a pointer to the matched document
	// sorting the collection by created at (the most recent one at first)
	ptr, err := collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer ptr.Close(ctx)

	// decode bson
	var jobs []gateway.Job
	for ptr.Next(ctx) {
		var job gateway.Job
		if err := ptr.Decode(&job); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Println(job) // or append to a slice
		jobs = append(jobs, job)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (s *Scheduler) HandleGetJobById(w http.ResponseWriter, r *http.Request) {
	jobIDStr := r.URL.Query().Get("job_id")
	if jobIDStr == "" {
		http.Error(w, "job_id parameter required", http.StatusBadRequest)
		return
	}

	// mongoDb object
	objID, err := primitive.ObjectIDFromHex(jobIDStr)
	if err != nil {
		http.Error(w, "invalid job_id format", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	collection := s.db.Collection("jobs")

	var job gateway.Job
	err = collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&job)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch job", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}
