package main

import (
	"distributed-job-scheduler/pkg/queue"
	"log"
	"net/http"
)

// a single instance of the queue

func main() {
	queueService := queue.NewQueueService()

	http.HandleFunc("/queue/push", queueService.HandlePush)
	http.HandleFunc("/queue/pop", queueService.HandlePop)
	http.HandleFunc("/queue/peek", queueService.HandlePeek)
	http.HandleFunc("/queue", queueService.HandleGetAll)
	http.HandleFunc("/stats", queueService.HandleStats)

	log.Println("Queue running on port 6000")
	log.Fatal(http.ListenAndServe(":6000", nil))
}
