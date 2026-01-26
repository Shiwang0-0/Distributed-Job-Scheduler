package main

import (
	"context"
	"distributed-job-scheduler/pkg/shutdown"
	workerqueuecoordinator "distributed-job-scheduler/pkg/worker-queue-coordinator"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")

	port := flag.String("port", "3000", "Coordinator port")
	mongoURI := flag.String("mongo-uri", getEnv("MONGO_URI", "mongodb://localhost:27017"), "MongoDB URI")
	dbName := flag.String("db", getEnv("MONGO_DB", "job_scheduler"), "MongoDB database name")
	normalQueues := flag.String("normal", "", "Comma-separated normal queue ports")
	highQueues := flag.String("high", "", "Comma-separated high-priority queue ports")

	flag.Parse()

	if *normalQueues == "" && *highQueues == "" {
		log.Fatal("At least one queue must be provided via --normal or --high")
	}

	var allQueuePorts []string

	if *normalQueues != "" {
		allQueuePorts = append(allQueuePorts, strings.Split(*normalQueues, ",")...)
	}

	if *highQueues != "" {
		allQueuePorts = append(allQueuePorts, strings.Split(*highQueues, ",")...)
	}

	coord, err := workerqueuecoordinator.NewCoordinator(
		*mongoURI,
		*dbName,
		*port,
		allQueuePorts,
	)
	if err != nil {
		log.Fatalf("Failed to start coordinator: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/coordinator/request-assignment", coord.HandleRequestAssignment)
	mux.HandleFunc("/coordinator/heartbeat", coord.HandleHeartbeat)
	mux.HandleFunc("/coordinator/release-assignment", coord.HandleReleaseAssignment)
	mux.HandleFunc("/coordinator/stats", coord.HandleStats)

	log.Printf("Coordinator listening on port %s", *port)
	log.Printf("Queues registered: %v", allQueuePorts)

	server := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		log.Printf("Coordinator running on port %s\n", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Coordinator failed: %v", err)
		}
	}()

	shutdown.Listen(func(ctx context.Context) {
		log.Println("Shutting down Coordinator...")
		server.Shutdown(ctx)
		coord.Shutdown()
		log.Println("Coordinator stopped gracefully")
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
