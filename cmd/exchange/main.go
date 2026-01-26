package main

import (
	"context"
	"distributed-job-scheduler/pkg/exchange"
	"distributed-job-scheduler/pkg/shutdown"
	"flag"
	"log"
	"net/http"
	"strings"
)

func main() {

	port := flag.String("port", "3000", "Coordinator port")
	normalQueues := flag.String("normal", "", "Comma-separated normal queue ports")
	highQueues := flag.String("high", "", "Comma-separated high-priority queue ports")

	flag.Parse()

	if *port == "" {
		log.Fatal("Port is required. Usage: ./exchange -port=2001")
	}

	normalQueuePorts := strings.Split(*normalQueues, ",")
	highPriorityQueuePorts := strings.Split(*highQueues, ",")

	exc := exchange.NewExchange(*port, normalQueuePorts, highPriorityQueuePorts)

	mux := http.NewServeMux()
	mux.HandleFunc("/exchange/route", exc.HandleRouteJob)
	mux.HandleFunc("/exchange/stats", exc.HandleStats)

	server := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		log.Printf("Exchange running on port %s\n", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Exchange failed: %v", err)
		}
	}()

	shutdown.Listen(func(ctx context.Context) {
		log.Println("Shutting down Exchange...")
		server.Shutdown(ctx)
		exc.Shutdown()
		log.Println("Exchange stopped gracefully")
	})
}
