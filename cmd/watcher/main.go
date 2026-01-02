package main

import (
	"distributed-job-scheduler/pkg/watcher"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env") // with respect to pwd

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

	watcher, err := watcher.NewWatcher(mongoURI, dbName, port)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/stats", watcher.HandleStats)

	log.Println("Watcher running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
