package workerqueuecoordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func NewCoordinator(mongoURI, dbName, port string, queuePorts []string) (*Coordinator, error) {
	ctx, cancle := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancle()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	coordinatorID := fmt.Sprintf("coordinator-%s", port)
	log.Printf("Coordinator:%s Connected to MongoDB", coordinatorID)

	db := client.Database(dbName)

	// index on worker_id for fast lookups
	collection := db.Collection("worker_assignments")
	_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "worker_id", Value: 1}},
	})
	if err != nil {
		log.Printf("Coordinator:%s Warning: Failed to create index: %v", coordinatorID, err)
	}

	coordinator := &Coordinator{
		client:        client,
		db:            db,
		coordinatorID: coordinatorID,
		port:          port,
		allQueuePorts: queuePorts,
		stats:         &CoordinatorStats{},
		stopChan:      make(chan struct{}),
	}

	// Start background tasks
	coordinator.startStaleWorkerRecovery()

	// monitor queue coordination (less queues, failed queues)
	coordinator.monitorQueueCoverage()

	log.Printf("Coordinator:%s Started managing %d queues", coordinatorID, len(queuePorts))

	return coordinator, nil
}

func (c *Coordinator) startStaleWorkerRecovery() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(HeartbeatTimeout)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.recoverStaleWorkers()
			case <-c.stopChan:
				return
			}
		}
	}()
}

// the workers which have stopped sending heart beat, are now stale
func (c *Coordinator) recoverStaleWorkers() {
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	staleThreshold := time.Now().Add(-HeartbeatTimeout)

	filter := bson.M{
		"status":         "active",
		"last_heartbeat": bson.M{"$lt": staleThreshold},
	}

	update := bson.M{
		"$set": bson.M{
			"status": "stale",
		},
	}

	result, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		log.Printf("Coordinator:%s Error recovering stale workers: %v", c.coordinatorID, err)
	} else if result.ModifiedCount > 0 {
		log.Printf("Coordinator:%s Recovered %d stale workers", c.coordinatorID, result.ModifiedCount)
		c.stats.IncrementStaleRecovered(result.ModifiedCount)
	}
}

// --------------------------------------------------------------------------------------

func (c *Coordinator) HandleRequestAssignment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkerID string `json:"worker_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if req.WorkerID == "" {
		http.Error(w, "worker_id required", http.StatusBadRequest)
		return
	}

	// Check if worker already has an assignment
	existingAssignment, err := c.getWorkerAssignment(req.WorkerID)
	if err == nil && existingAssignment != nil && existingAssignment.Status == "active" {
		// Worker has a queue assigned, Extend lease if still valid
		if time.Now().Before(existingAssignment.LeaseExpiresAt) {
			c.extendLease(req.WorkerID)
			log.Printf("Coordinator:%s Extended lease for worker %s on queue %s",
				c.coordinatorID, req.WorkerID, existingAssignment.QueuePort)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(existingAssignment)
			return
		}
	}

	// Assign new queue based on load balancing
	queuePort, err := c.assignQueueToWorker(req.WorkerID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to assign queue: %v", err), http.StatusInternalServerError)
		return
	}

	assignment := &WorkerAssignment{
		WorkerID:       req.WorkerID,
		QueuePort:      queuePort,
		AssignedAt:     time.Now(),
		LastHeartbeat:  time.Now(),
		LeaseExpiresAt: time.Now().Add(LeaseDuration),
		Status:         "active",
	}

	// Store in MongoDB
	if err := c.saveAssignment(assignment); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save assignment: %v", err), http.StatusInternalServerError)
		return
	}

	c.stats.IncrementAssignments()
	log.Printf("Coordinator:%s Assigned worker %s to queue %s",
		c.coordinatorID, req.WorkerID, queuePort)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assignment)
}

// Hash & Load-balanced queue assignment
func (c *Coordinator) assignQueueToWorker(workerID string) (string, error) {
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	// Count active workers per queue
	queueLoadMap := make(map[string]int)
	for _, queuePort := range c.allQueuePorts {
		queueLoadMap[queuePort] = 0
	}

	// Get all active assignments
	activeAssignments, err := collection.Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return "", err
	}
	defer activeAssignments.Close(ctx)

	var assignments []WorkerAssignment
	if err := activeAssignments.All(ctx, &assignments); err != nil {
		return "", err
	}

	// Count workers per queue
	for _, assignment := range assignments {
		if time.Now().Before(assignment.LeaseExpiresAt) {
			queueLoadMap[assignment.QueuePort]++
		}
	}

	numQueues := len(c.allQueuePorts)
	if numQueues == 0 {
		return "", fmt.Errorf("no queues available")
	}

	// each worker based on the worker id has a different starting point because of hash
	// in other case if workers would have joined concurrently, they would have grab the same queue (minimum load)
	// but now each worker has a different starting point

	// also there must be more workers than queues, otherwise some queues will be left out to be polled by worker

	hash := hashString(workerID)
	startIdx := int(hash % uint32(numQueues))

	minLoad := math.MaxInt
	selectedQueue := ""

	for i := 0; i < numQueues; i++ {
		idx := (startIdx + i) % numQueues
		queuePort := c.allQueuePorts[idx]
		load := queueLoadMap[queuePort]

		if load < minLoad {
			minLoad = load
			selectedQueue = queuePort
		}
	}

	if selectedQueue == "" {
		selectedQueue = c.allQueuePorts[0] // Fallback to first queue
	}

	log.Printf("Coordinator:%s Queue load distribution: %v, selected: %s", c.coordinatorID, queueLoadMap, selectedQueue)

	return selectedQueue, nil
}

func (c *Coordinator) HandleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkerID string `json:"worker_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if req.WorkerID == "" {
		http.Error(w, "worker_id required", http.StatusBadRequest)
		return
	}

	// Update heartbeat timestamp
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	now := time.Now()
	result, err := collection.UpdateOne(
		ctx,
		bson.M{"worker_id": req.WorkerID, "status": "active"},
		bson.M{
			"$set": bson.M{
				"last_heartbeat":   now,
				"lease_expires_at": now.Add(LeaseDuration),
			},
		},
	)

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update heartbeat: %v", err), http.StatusInternalServerError)
		return
	}

	if result.MatchedCount == 0 {
		http.Error(w, "Worker assignment not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "heartbeat_received"})
}

// Worker releases its queue assignment
func (c *Coordinator) HandleReleaseAssignment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkerID string `json:"worker_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if err := c.releaseWorkerAssignment(req.WorkerID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to release assignment: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Coordinator:%s Released worker %s assignment", c.coordinatorID, req.WorkerID)
	w.WriteHeader(http.StatusOK)
}

func (c *Coordinator) monitorQueueCoverage() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.checkQueueCoverage()
			case <-c.stopChan:
				return
			}
		}
	}()
}

// Warning for Less Queues than Workers

func (c *Coordinator) checkQueueCoverage() {
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	queueLoadMap := make(map[string]int)
	for _, queuePort := range c.allQueuePorts {
		queueLoadMap[queuePort] = 0
	}

	cursor, err := collection.Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	var assignments []WorkerAssignment
	cursor.All(ctx, &assignments)

	for _, assignment := range assignments {
		if time.Now().Before(assignment.LeaseExpiresAt) {
			queueLoadMap[assignment.QueuePort]++
		}
	}

	var uncoveredQueues []string
	for _, queuePort := range c.allQueuePorts {
		if queueLoadMap[queuePort] == 0 {
			uncoveredQueues = append(uncoveredQueues, queuePort)
		}
	}

	if len(uncoveredQueues) > 0 {
		log.Printf("Coordinator:%s WARNING: %d queues have NO workers: %v",
			c.coordinatorID, len(uncoveredQueues), uncoveredQueues)

		log.Printf("Coordinator:%s Recommendation: Add at least %d more workers",
			c.coordinatorID, len(uncoveredQueues))
	} else {
		log.Printf("Coordinator:%s All %d queues have worker coverage",
			c.coordinatorID, len(c.allQueuePorts))
	}
}
