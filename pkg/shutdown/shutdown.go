package shutdown

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"
)

// Listen listens for OS interrupt signals and calls the provided cleanup function.
func Listen(cleanup func(ctx context.Context)) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done() // wait for signal
	log.Println("Shutdown signal received")

	// Use a context with timeout for cleanup
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if cleanup != nil {
		cleanup(shutdownCtx)
	}
}
