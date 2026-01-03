package main

import (
	"distributed-job-scheduler/pkg/worker"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env") // with respect to pwd

	port := os.Getenv("PORT")
	if port == "" {
		port = "7001"
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	dbName := os.Getenv("MONGO_DB")
	if dbName == "" {
		dbName = "job_scheduler"
	}

	concurrency := 5
	if c := os.Getenv("CONCURRENCY"); c != "" {
		fmt.Sscanf(c, "%d", &concurrency)
	}

	worker, err := worker.NewWorker(mongoURI, dbName, port, concurrency)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/stats", worker.HandleStats)

	log.Println("Worker running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
