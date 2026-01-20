package main

import (
	"context"
	"distributed-job-scheduler/pkg/scheduler"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env") // with respect to pwd

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	dbName := os.Getenv("MONGO_DB")
	if dbName == "" {
		dbName = "job_scheduler"
	}

	scheduler, err := scheduler.NewScheduler(mongoURI, dbName, port)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start leader election process
	scheduler.StartLeaderElection(ctx)

	http.HandleFunc("/health", scheduler.HandleHealth)
	http.HandleFunc("/schedule", scheduler.HandleSchedule)
	http.HandleFunc("/jobs", scheduler.HandleGetJobs)
	http.HandleFunc("/job", scheduler.HandleGetJobById)

	server := &http.Server{
		Addr: ":" + port,
	}

	go func() {
		log.Printf("Scheduler running on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Shutdown scheduler (releases lease, stops goroutines)
	if err := scheduler.Shutdown(ctx); err != nil {
		log.Printf("Scheduler shutdown error: %v", err)
	}

	// Shutdown HTTP server
	if err := scheduler.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
