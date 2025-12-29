package scheduler

import (
	"encoding/json"
	"log"
	"net/http"
)

type Scheduler struct{}

func NewScheduler() *Scheduler {
	return &Scheduler{}
}

func (s *Scheduler) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func (s *Scheduler) HandleSchedule(w http.ResponseWriter, r *http.Request) {
	log.Println("Job received")

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "scheduled",
	})
}
