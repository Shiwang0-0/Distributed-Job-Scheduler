package gateway

import (
	"bytes"
	"distributed-job-scheduler/pkg/loadbalancer"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func NewAPIGateway(lb *loadbalancer.LoadBalancer) *APIGateway {
	return &APIGateway{lb: lb}
}

func (gw *APIGateway) HandleCreateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var job Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate job ID
	if job.Id == "" {
		job.Id = fmt.Sprintf("job_%d", time.Now().UnixNano())
	}

	service := gw.lb.GetScheduler()
	if service == nil {
		http.Error(w, "No healthy scheduler services available", http.StatusServiceUnavailable)
		return
	}

	jobData, _ := json.Marshal(job)
	res, err := http.Post(service.Url+"/schedule", "application/json", bytes.NewBuffer(jobData))
	if err != nil {
		log.Printf("Error forwarding to scheduler %s: %v", service.Url, err)
		http.Error(w, "Failed to schedule job", http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	w.Write(body)

	log.Printf("Job %s routed to %s", job.Id, service.Url)
}
