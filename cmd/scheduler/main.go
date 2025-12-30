package main

import (
	"distributed-job-scheduler/pkg/scheduler"
	"log"
	"net/http"
	"os"

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

	http.HandleFunc("/health", scheduler.HandleHealth)
	http.HandleFunc("/schedule", scheduler.HandleSchedule)

	log.Println("Scheduler running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
