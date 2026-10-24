package domain

import (
	"log"
	"net/http"
	"strings"

	"imgproxy/internal/config"
)

type Whitelist struct {
	domains map[string]string
}

func CreateWhitelist() *Whitelist {
	wl := &Whitelist{domains: make(map[string]string)}
	entries := config.Strings("DOMAINS")
	if len(entries) == 0 {
		log.Println("No whitelisted domains found in environment variable 'DOMAINS'")
		return wl
	}
	for _, entry := range entries {
		name, domain, found := strings.Cut(entry, "=")
		if !found {
			domain = name
		}
		wl.domains[name] = domain
	}
	log.Printf("Allowed domains: %v", wl.domains)
	return wl
}

func (wl *Whitelist) Resolve(name string) (string, bool) {
	if len(wl.domains) == 0 {
		return name, name != ""
	}
	domain, ok := wl.domains[name]
	return domain, ok
}

func (wl *Whitelist) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		name := query.Get("d")
		domain, ok := wl.Resolve(name)
		if !ok {
			if name == "" {
				http.Error(w, "Missing source domain", http.StatusBadRequest)
			} else {
				http.Error(w, "Forbidden domain", http.StatusForbidden)
			}
			return
		}
		if domain != name {
			query.Set("d", domain)
			r2 := r.Clone(r.Context())
			r2.URL.RawQuery = query.Encode()
			r = r2
		}
		next.ServeHTTP(w, r)
	})
}
