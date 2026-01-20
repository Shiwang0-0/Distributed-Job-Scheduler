package scheduler

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (s *Scheduler) StartLeaderElection(ctx context.Context) {
	s.wg.Add(1)
	go s.leaderElectionLoop(ctx)
}

func (s *Scheduler) leaderElectionLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(LeaderCheckDelay)
	defer ticker.Stop()

	log.Printf("Scheduler:%s Starting leader election process", s.port)

	// someone pitches to become the leader
	s.tryAcquireLease(ctx)

	for {
		select {
		case <-ticker.C:
			s.leaderMu.RLock()
			isLeader := s.isLeader
			s.leaderMu.RUnlock()

			if isLeader {
				// renew lease if we're the leader
				if !s.renewLease(ctx) {
					log.Printf("Scheduler:%s Failed to renew lease, stepping down as leader", s.port)
					s.stepDown()
				}
			} else {
				// try to pitch for becoming the leader if we're not
				s.tryAcquireLease(ctx)
			}

		case <-s.stopChan:
			log.Printf("Scheduler:%s Leader election stopped", s.port)
			s.releaseLease(ctx)
			return
		}
	}
}
func (s *Scheduler) tryAcquireLease(ctx context.Context) bool {
	collection := s.db.Collection("leader_lease")
	now := time.Now()

	// Use a fixed document ID to ensure only ONE lease document exists
	leaseDocID := "leader"

	// Filter: match the fixed ID AND (lease expired OR doesn't exist)
	filter := bson.M{
		"_id": leaseDocID,
		"$or": []bson.M{
			{"expires_at": bson.M{"$lt": now}},      // Expired lease
			{"leader_id": bson.M{"$exists": false}}, // No leader yet
		},
	}

	update := bson.M{
		"$set": bson.M{
			"leader_id":   s.instanceId,
			"acquired_at": now,
			"expires_at":  now.Add(LeaseDuration),
			"updated_at":  now,
		},
		"$setOnInsert": bson.M{
			"_id": leaseDocID, // Ensure fixed ID on first insert
		},
	}

	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var lease LeaderLease
	err := collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&lease)
	if err != nil {
		// Only log unexpected errors
		if err != mongo.ErrNoDocuments && !mongo.IsDuplicateKeyError(err) {
			log.Printf("Scheduler:%s Unexpected error acquiring lease: %v", s.port, err)
		}
		return false
	}

	if lease.LeaderId != s.instanceId {
		return false
	}

	s.leaderMu.Lock()
	wasLeader := s.isLeader
	s.isLeader = true
	s.leaderMu.Unlock()

	if !wasLeader {
		log.Printf("Scheduler:%s Became LEADER", s.port)
		s.wg.Add(1)
		go s.CronTicker()
	}

	return true
}

func (s *Scheduler) renewLease(ctx context.Context) bool {
	collection := s.db.Collection("leader_lease")
	now := time.Now()

	filter := bson.M{
		"leader_id": s.instanceId,
	}

	update := bson.M{
		"$set": bson.M{
			"expires_at": now.Add(LeaseDuration),
			"updated_at": now,
		},
	}

	result, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Printf("Scheduler:%s Failed to renew lease: %v", s.port, err)
		return false
	}

	if result.MatchedCount == 0 {
		log.Printf("Scheduler:%s Lost leadership (lease taken by another instance)", s.port)
		return false
	}

	return true
}

func (s *Scheduler) releaseLease(ctx context.Context) {
	s.leaderMu.RLock()
	isLeader := s.isLeader
	s.leaderMu.RUnlock()

	if !isLeader {
		return
	}

	collection := s.db.Collection("leader_lease")

	filter := bson.M{
		"leader_id": s.instanceId,
	}

	_, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		log.Printf("Scheduler:%s Failed to release lease: %v", s.port, err)
	} else {
		log.Printf("Scheduler:%s Released leadership lease", s.port)
	}

	s.stepDown()
}

func (s *Scheduler) stepDown() {
	s.leaderMu.Lock()
	s.isLeader = false
	s.leaderMu.Unlock()
}

// IsLeader returns whether this instance is the current leader
func (s *Scheduler) IsLeader() bool {
	s.leaderMu.RLock()
	defer s.leaderMu.RUnlock()
	return s.isLeader
}

func (s *Scheduler) Shutdown(ctx context.Context) error {
	log.Printf("Scheduler:%s Shutting down...", s.port)

	s.releaseLease(ctx)

	// Close stopChan safely
	select {
	case <-s.stopChan:
		// Already closed
	default:
		close(s.stopChan)
	}

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("Scheduler:%s Graceful shutdown complete", s.port)
	case <-ctx.Done():
		log.Printf("Scheduler:%s Shutdown timeout, forcing exit", s.port)
	}

	return s.client.Disconnect(ctx)
}
