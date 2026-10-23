package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"imgproxy/internal/config"
	"imgproxy/internal/domain"
	"imgproxy/internal/image"
	"imgproxy/internal/middleware"
)

func getMaxWorker() int {
	defaultMax := max(runtime.NumCPU(), 1)
	return config.Int("MAX_WORKERS", defaultMax)
}

func main() {
	// Allow GOMAXPROCS override
	if n := config.Int("GOMAXPROCS", 0); n > 0 {
		runtime.GOMAXPROCS(n)
	}

	maxWorkers := getMaxWorker()

	maxReq := config.Int("MAX_HTTP_CONNS", max(4, maxWorkers*2))

	mux := http.NewServeMux()
	handler := middleware.LimitMiddleware(mux, maxReq, 25*time.Second, "/health-z")
	wl := domain.CreateWhitelist()
	// Used as-is, e.g. "/x" serves images under /x/; empty serves from root
	prefix := config.String("IMAGE_PATH_PREFIX", "") + "/"
	mux.Handle(prefix, http.StripPrefix(prefix, wl.Guard(http.HandlerFunc(image.Handler()))))
	mux.HandleFunc("/health-z", healthCheck)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	idleConnsClosed := make(chan struct{})
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		<-signals
		log.Println("Shutdown signal received, shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("HTTP server Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	log.Printf("Server starting on %s (PREFIX=%s, MAX_WORKERS=%d, VIPS_CONCURRENCY=%s)\n",
		srv.Addr, prefix, maxWorkers, os.Getenv("VIPS_CONCURRENCY"))
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe(): %v", err)
	}
	<-idleConnsClosed
	log.Println("Server stopped")
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
