package main

import (
	"context"
	"distributed-job-scheduler/pkg/scheduler"
	"distributed-job-scheduler/pkg/shutdown"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")

	port := flag.String("port", "3000", "Coordinator port")
	mongoURI := flag.String("mongo-uri", getEnv("MONGO_URI", "mongodb://localhost:27017"), "MongoDB URI")
	dbName := flag.String("db", getEnv("MONGO_DB", "job_scheduler"), "MongoDB database name")

	flag.Parse()

	scheduler, err := scheduler.NewScheduler(*mongoURI, *dbName, *port)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start leader election process
	scheduler.StartLeaderElection(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", scheduler.HandleHealth)
	mux.HandleFunc("/schedule", scheduler.HandleSchedule)
	mux.HandleFunc("/jobs", scheduler.HandleGetJobs)
	mux.HandleFunc("/job", scheduler.HandleGetJobById)

	server := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		log.Printf("Scheduler running on port %s", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Scheduler failed: %v", err)
		}
	}()

	shutdown.Listen(func(ctx context.Context) {
		log.Println("Shutting down Scheduler...")
		server.Shutdown(ctx)
		scheduler.Shutdown(ctx)
		log.Println("Scheduler stopped gracefully")
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
