package web

import (
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
)

// corsMiddleware adds CORS headers for API requests from Amplify-hosted frontends.
// Allows webmail.{domain} origins for all configured domains.
func corsMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Only apply CORS to /api/* routes
		if origin != "" && strings.HasPrefix(r.URL.Path, "/api/") {
			if isAllowedOrigin(origin, cfg) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "3600")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, cfg *config.Config) bool {
	for _, d := range cfg.GetDomains() {
		if origin == "https://webmail."+d || origin == "https://mail."+d {
			return true
		}
	}
	return false
}
