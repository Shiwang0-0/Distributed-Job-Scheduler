package main

import (
	"log"
	"net/http"

	"distributed-job-scheduler/pkg/gateway"
	"distributed-job-scheduler/pkg/loadbalancer"
)

func main() {
	serviceURLs := []string{
		"http://localhost:8081",
		"http://localhost:8082",
		"http://localhost:8083",
	}

	lb := loadbalancer.NewLoadBalancer(serviceURLs)
	apiGateway := gateway.NewAPIGateway(lb)

	http.HandleFunc("/api/jobs", apiGateway.HandleCreateJob)
	http.HandleFunc("/jobs", apiGateway.HandleGetJobs)
	http.HandleFunc("/job", apiGateway.HandleGetJobById)

	port := ":8080"
	log.Println("API Gateway running on", port)

	log.Fatal(http.ListenAndServe(port, nil))
}
