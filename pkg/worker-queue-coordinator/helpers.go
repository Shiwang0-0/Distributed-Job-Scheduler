package workerqueuecoordinator

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"hash/fnv"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func hashString(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func (c *Coordinator) getWorkerAssignment(workerID string) (*WorkerAssignment, error) {
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	var assignment WorkerAssignment
	err := collection.FindOne(ctx, bson.M{"worker_id": workerID}).Decode(&assignment)
	if err != nil {
		return nil, err
	}

	return &assignment, nil
}

func (c *Coordinator) saveAssignment(assignment *WorkerAssignment) error {
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(
		ctx,
		bson.M{"worker_id": assignment.WorkerID},
		bson.M{"$set": assignment},
		opts,
	)

	return err
}

func (c *Coordinator) extendLease(workerID string) error {
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	now := time.Now()
	_, err := collection.UpdateOne(
		ctx,
		bson.M{"worker_id": workerID},
		bson.M{
			"$set": bson.M{
				"last_heartbeat":   now,
				"lease_expires_at": now.Add(LeaseDuration),
			},
		},
	)

	return err
}

func (c *Coordinator) releaseWorkerAssignment(workerID string) error {
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"worker_id": workerID},
		bson.M{
			"$set": bson.M{
				"status": "released",
			},
		},
	)

	return err
}

func (c *Coordinator) HandleStats(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	collection := c.db.Collection("worker_assignments")

	// Count active workers
	activeCount, _ := collection.CountDocuments(ctx, bson.M{"status": "active"})

	stats := c.stats.GetStats()
	stats["coordinator_id"] = c.coordinatorID
	stats["active_workers"] = activeCount
	stats["managed_queues"] = len(c.allQueuePorts)
	stats["queue_ports"] = c.allQueuePorts

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (c *Coordinator) Shutdown() {
	close(c.stopChan)
	c.wg.Wait()
	c.client.Disconnect(context.Background())
	log.Printf("Coordinator:%s Shutdown complete", c.coordinatorID)
}
