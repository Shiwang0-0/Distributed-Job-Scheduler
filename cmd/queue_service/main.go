package main

import (
	"context"
	"distributed-job-scheduler/pkg/queue"
	"distributed-job-scheduler/pkg/shutdown"
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {

	port := flag.String("port", "", "Queue port (required)")
	queueType := flag.String("type", "normal", "Queue type: normal or high")
	queueIndex := flag.Int("index", 0, "Queue index for ID generation")
	flag.Parse()

	if *port == "" {
		log.Fatal("Port is required. Usage: ./queue -port 7001 -type normal -index 0")
	}

	// Generate queue ID
	var queueID string
	if *queueType == "high" {
		queueID = fmt.Sprintf("high_priority_queue_%d", *queueIndex)
	} else {
		queueID = fmt.Sprintf("normal_queue_%d", *queueIndex)
	}

	log.Printf("Starting Queue Service: %s on port %s", queueID, *port)

	queueService := queue.NewQueueService(queueID, *port)

	mux := http.NewServeMux()
	mux.HandleFunc("/queue/push", queueService.HandlePush)
	mux.HandleFunc("/queue/delete", queueService.HandleDelete)
	mux.HandleFunc("/queue/lease", queueService.HandleLease)
	mux.HandleFunc("/queue/release-lease", queueService.HandleReleaseLease)
	mux.HandleFunc("/queue/peek", queueService.HandlePeek)
	mux.HandleFunc("/queue/all", queueService.HandleGetAll)
	mux.HandleFunc("/queue/stats", queueService.HandleStats)

	server := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		log.Printf("Queue %s running on port %s", queueID, *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Queue failed: %v", err)
		}
	}()

	shutdown.Listen(func(ctx context.Context) {
		log.Printf("Shutting down queue %s...", queueID)
		server.Shutdown(ctx)
		queueService.Shutdown()
		log.Printf("Queue %s stopped gracefully", queueID)
	})

}
