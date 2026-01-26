package main

import (
	"context"
	"distributed-job-scheduler/pkg/shutdown"
	"distributed-job-scheduler/pkg/watcher"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env")

	port := flag.String("port", "3000", "Coordinator port")
	mongoURI := flag.String("mongo-uri", getEnv("MONGO_URI", "mongodb://localhost:27017"), "MongoDB URI")
	dbName := flag.String("db", getEnv("MONGO_DB", "job_scheduler"), "MongoDB database name")
	exchangePortsFlag := flag.String("exchanges", "", "Comma-separated exchange ports (2001,2002)")

	flag.Parse()

	if *exchangePortsFlag == "" {
		log.Fatal("No exchanges provided. Use --exchanges")
	}

	exchangePorts := strings.Split(*exchangePortsFlag, ",")

	watcher, err := watcher.NewWatcher(*mongoURI, *dbName, *port, exchangePorts)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stats", watcher.HandleStats)

	server := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		log.Printf("Watcher running on port %s", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Watcher server failed: %v", err)
		}
	}()

	// shutdown
	shutdown.Listen(func(ctx context.Context) {
		log.Println("Shutting down watcher...")
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
		watcher.Shutdown()
		log.Println("Watcher stopped gracefully")
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
