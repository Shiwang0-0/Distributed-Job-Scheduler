package main

import (
	"distributed-job-scheduler/pkg/scheduler"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	svc := scheduler.NewScheduler()

	http.HandleFunc("/health", svc.HandleHealth)
	http.HandleFunc("/schedule", svc.HandleSchedule)

	log.Println("Scheduler running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
