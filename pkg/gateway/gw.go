package gateway

import (
	"bytes"
	"distributed-job-scheduler/pkg/loadbalancer"
	"encoding/json"
	"io"
	"log"
	"net/http"
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)

	log.Printf("Job routed to %s", service.Url)
}

func (gw *APIGateway) HandleGetJobs(w http.ResponseWriter, r *http.Request) {
	service := gw.lb.GetScheduler()
	if service == nil {
		http.Error(w, "No healthy scheduler available", http.StatusServiceUnavailable)
		return
	}

	// ?status="pending"
	url := service.Url + "/jobs" + "?" + r.URL.RawQuery
	res, err := http.Get(url)
	if err != nil {
		http.Error(w, "Failed to fetch jobs", http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}

func (gw *APIGateway) HandleGetJobById(w http.ResponseWriter, r *http.Request) {
	service := gw.lb.GetScheduler()
	if service == nil {
		http.Error(w, "No healthy scheduler available", http.StatusServiceUnavailable)
		return
	}

	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}
	url := service.Url + "/job?job_id=" + jobID

	res, err := http.Get(url)
	if err != nil {
		http.Error(w, "Failed to fetch jobs", http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}
