package middleware

import (
	"log"
	"net/http"
	"time"
)

func LimitMiddleware(next http.Handler, maxReq int, timeout time.Duration, skipPaths ...string) http.Handler {
	sem := make(chan struct{}, maxReq)
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := skip[r.URL.Path]; ok {
			next.ServeHTTP(w, r)
			return
		}
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			next.ServeHTTP(w, r)
			return
		default:
			log.Println("Slot is not available yet, waiting...")
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				next.ServeHTTP(w, r)
			case <-r.Context().Done():
			case <-timer.C:
				log.Println("Timeout, Server busy")
				http.Error(w, "Server busy", http.StatusServiceUnavailable)
			}
		}
	})
}
