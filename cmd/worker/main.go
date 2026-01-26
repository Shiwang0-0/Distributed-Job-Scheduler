package main

import (
	"context"
	"distributed-job-scheduler/pkg/shutdown"
	"distributed-job-scheduler/pkg/worker"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env") // with respect to pwd

	port := flag.String("port", "3000", "Coordinator port")
	mongoURI := flag.String("mongo-uri", getEnv("MONGO_URI", "mongodb://localhost:27017"), "MongoDB URI")
	dbName := flag.String("db", getEnv("MONGO_DB", "job_scheduler"), "MongoDB database name")
	coordinatorPort := flag.String("coordinator", "3000", "Coordinator port")
	concurrency := flag.Int("concurrency", 5, "Worker concurrency")

	flag.Parse()

	w, err := worker.NewWorker(*mongoURI, *dbName, *port, *coordinatorPort, *concurrency)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stats", w.HandleStats)

	server := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		log.Printf("Worker running on port %s", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Worker server failed: %v", err)
		}
	}()

	// shutdown
	shutdown.Listen(func(ctx context.Context) {
		log.Println("Shutting down worker...")
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
		w.Shutdown()
		log.Println("Worker stopped gracefully")
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
