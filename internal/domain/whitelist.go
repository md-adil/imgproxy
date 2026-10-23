package domain

import (
	"log"
	"net/http"

	"imgproxy/internal/config"
)

// Whitelist holds the set of allowed domains loaded from the environment.
type Whitelist struct {
	allowed map[string]bool
}

// NewWhitelist reads WHITELISTED_DOMAINS from the environment and returns a Whitelist.
// If the env var is empty, all domains are allowed.
func CreateWhitelist() *Whitelist {
	wl := &Whitelist{allowed: make(map[string]bool)}
	domains := config.Strings("WHITELISTED_DOMAINS")
	if len(domains) == 0 {
		log.Println("WHITELISTED_DOMAINS not set — all domains are allowed")
		return wl
	}
	for _, domain := range domains {
		wl.allowed[domain] = true
	}
	log.Printf("Whitelisted domains: %v", wl.allowed)
	return wl
}

// IsAllowed reports whether the given domain is permitted.
// If no domains are configured, all domains are allowed.
func (wl *Whitelist) IsAllowed(domain string) bool {
	if len(wl.allowed) == 0 {
		return true
	}
	return wl.allowed[domain]
}

// Guard wraps next and rejects requests whose ?d= query param is not whitelisted.
func (wl *Whitelist) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := r.URL.Query().Get("d")
		if d == "" {
			http.Error(w, "Missing source domain", http.StatusBadRequest)
			return
		}
		if !wl.IsAllowed(d) {
			http.Error(w, "Forbidden domain", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
