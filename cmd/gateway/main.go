package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"strings"

	"distributed-job-scheduler/pkg/gateway"
	"distributed-job-scheduler/pkg/loadbalancer"
	"distributed-job-scheduler/pkg/shutdown"
)

func main() {
	port := flag.String("port", "8000", "Gateway port")

	schedulerPorts := flag.String(
		"schedulers",
		"",
		"Comma-separated scheduler ports (e.g. 8081,8082,8083)",
	)

	flag.Parse()

	if *schedulerPorts == "" {
		log.Fatal("No schedulers provided. Use --schedulers")
	}

	// Scheduler URLs
	var serviceURLs []string
	for _, p := range strings.Split(*schedulerPorts, ",") {
		serviceURLs = append(serviceURLs, "http://localhost:"+p)
	}

	log.Printf("Gateway running on port %s", *port)
	log.Printf("Schedulers registered: %v", serviceURLs)

	lb := loadbalancer.NewLoadBalancer(serviceURLs)
	apiGateway := gateway.NewAPIGateway(lb)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/jobs", apiGateway.HandleCreateJob)
	mux.HandleFunc("/jobs", apiGateway.HandleGetJobs)
	mux.HandleFunc("/job", apiGateway.HandleGetJobById)

	server := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		log.Printf("Gateway running on port %s", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Gateway failed: %v", err)
		}
	}()

	shutdown.Listen(func(ctx context.Context) {
		log.Println("Shutting down Gateway...")
		server.Shutdown(ctx)
		log.Println("Gateway stopped gracefully")
	})
}
