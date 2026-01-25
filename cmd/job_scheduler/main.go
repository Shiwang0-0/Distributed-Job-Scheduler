package main

import (
	"distributed-job-scheduler/pkg/exchange"
	"distributed-job-scheduler/pkg/queue"
	"distributed-job-scheduler/pkg/watcher"
	"distributed-job-scheduler/pkg/worker"
	workerqueuecoordinator "distributed-job-scheduler/pkg/worker-queue-coordinator"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	// Define ports
	exchangePorts := []string{"2001", "2002"}
	normalQueuePorts := []string{"7001", "7002", "7003", "7004"}
	highPriorityQueuePorts := []string{"7101", "7102"}
	allQueuePorts := append(normalQueuePorts, highPriorityQueuePorts...)
	coordinatorPort := "3000"
	watcherPorts := []string{"9001", "9002", "9003"}
	workerPorts := []string{"10001", "10002", "10003", "10004", "10005", "10006", "10007", "100008"}

	_ = godotenv.Load("./.env") // with respect to pwd

	port := os.Getenv("PORT")
	if port == "" {
		port = "9091"
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	dbName := os.Getenv("MONGO_DB")
	if dbName == "" {
		dbName = "job_scheduler"
	}

	log.Println("========================================")
	log.Println("Starting Distributed Job Scheduler")
	log.Println("========================================")

	// Start Coordinator
	log.Println("\n[1/6] Starting Coordinator...")
	coord, err := workerqueuecoordinator.NewCoordinator(mongoURI, dbName, coordinatorPort, allQueuePorts)
	if err != nil {
		log.Fatalf("Failed to start coordinator: %v", err)
	}

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/coordinator/request-assignment", coord.HandleRequestAssignment)
		mux.HandleFunc("/coordinator/heartbeat", coord.HandleHeartbeat)
		mux.HandleFunc("/coordinator/release-assignment", coord.HandleReleaseAssignment)
		mux.HandleFunc("/coordinator/stats", coord.HandleStats)

		log.Printf("Coordinator listening on port %s", coordinatorPort)
		http.ListenAndServe(":"+coordinatorPort, mux)
	}()

	// Start Normal Priority Queues
	log.Println("\n[2/6] Starting Normal Priority Queues...")
	for i, port := range normalQueuePorts {
		queueID := fmt.Sprintf("normal_queue_%d", i)
		queueService := queue.NewQueueService(queueID, port)

		go func(qs *queue.QueueService, p string) {
			mux := http.NewServeMux()
			mux.HandleFunc("/queue/push", qs.HandlePush)
			mux.HandleFunc("/queue/lease", qs.HandleLease)
			mux.HandleFunc("/queue/release-lease", qs.HandleReleaseLease)
			mux.HandleFunc("/queue/delete", qs.HandleDelete)
			mux.HandleFunc("/queue/peek", qs.HandlePeek)
			mux.HandleFunc("/queue/all", qs.HandleGetAll)
			mux.HandleFunc("/queue/stats", qs.HandleStats)

			log.Printf("Normal Queue %s listening on port %s", normalQueuePorts[i], p)
			http.ListenAndServe(":"+p, mux)
		}(queueService, port)
	}

	// Start High Priority Queues
	log.Println("\n[3/6] Starting High Priority Queues...")
	for i, port := range highPriorityQueuePorts {
		queueID := fmt.Sprintf("high_priority_queue_%d", i)
		queueService := queue.NewQueueService(queueID, port)

		go func(qs *queue.QueueService, p string) {
			mux := http.NewServeMux()
			mux.HandleFunc("/queue/push", qs.HandlePush)
			mux.HandleFunc("/queue/lease", qs.HandleLease)
			mux.HandleFunc("/queue/release-lease", qs.HandleReleaseLease)
			mux.HandleFunc("/queue/delete", qs.HandleDelete)
			mux.HandleFunc("/queue/peek", qs.HandlePeek)
			mux.HandleFunc("/queue/all", qs.HandleGetAll)
			mux.HandleFunc("/queue/stats", qs.HandleStats)

			log.Printf("High Priority Queue %s listening on port %s", highPriorityQueuePorts[i], p)
			http.ListenAndServe(":"+p, mux)
		}(queueService, port)
	}

	// Start Exchanges
	log.Println("\n[4/6] Starting Exchanges...")
	for _, port := range exchangePorts {
		exc := exchange.NewExchange(port, normalQueuePorts, highPriorityQueuePorts)

		go func(e *exchange.Exchange, p string) {
			mux := http.NewServeMux()
			mux.HandleFunc("/exchange/route", e.HandleRouteJob)
			mux.HandleFunc("/exchange/stats", e.HandleStats)

			log.Printf("Exchange listening on port %s", p)
			http.ListenAndServe(":"+p, mux)
		}(exc, port)
	}

	// Start Watchers
	log.Println("\n[5/6] Starting Watchers...")
	for _, port := range watcherPorts {
		w, err := watcher.NewWatcher(mongoURI, dbName, port, exchangePorts)
		if err != nil {
			log.Fatalf("Failed to start watcher on port %s: %v", port, err)
		}

		go func(watcher *watcher.Watcher, p string) {
			mux := http.NewServeMux()
			mux.HandleFunc("/watcher/stats", watcher.HandleStats)

			log.Printf("Watcher listening on port %s", p)
			http.ListenAndServe(":"+p, mux)
		}(w, port)
	}

	// Start Workers
	log.Println("\n[6/6] Starting Workers...")
	for _, port := range workerPorts {
		w, err := worker.NewWorker(mongoURI, dbName, port, coordinatorPort, 5)
		if err != nil {
			log.Fatalf("Failed to start worker on port %s: %v", port, err)
		}

		go func(worker *worker.Worker, p string) {
			mux := http.NewServeMux()
			mux.HandleFunc("/worker/stats", worker.HandleStats)

			log.Printf("Worker listening on port %s", p)
			http.ListenAndServe(":"+p, mux)
		}(w, port)
	}

	log.Println("\n========================================")
	log.Println(" All services started successfully!")
	log.Println("========================================")
	log.Printf("  Coordinator Stats: mux://localhost:%s/coordinator/stats\n", coordinatorPort)
	for _, port := range exchangePorts {
		log.Printf("  Exchange Stats: mux://localhost:%s/exchange/stats\n", port)
	}
	log.Println("\nPress Ctrl+C to shutdown...")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n\nShutting down gracefully...")
	coord.Shutdown()
	log.Println("Shutdown complete")
}
