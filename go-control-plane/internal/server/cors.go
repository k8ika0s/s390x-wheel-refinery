package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/config"
)

func withCORS(cfg config.Config, next http.Handler) http.Handler {
	if len(cfg.CORSOrigins) == 0 {
		return next
	}
	allowedOrigins := cfg.CORSOrigins
	allowedMethods := cfg.CORSMethods
	if len(allowedMethods) == 0 {
		allowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}
	allowedHeaders := cfg.CORSHeaders
	if len(allowedHeaders) == 0 {
		allowedHeaders = []string{"Content-Type", "Authorization", "X-Worker-Token"}
	}
	allowAny := containsString(allowedOrigins, "*")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAny || containsString(allowedOrigins, origin)) {
			allowOrigin := origin
			if allowAny && !cfg.CORSCredentials {
				allowOrigin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
			if cfg.CORSCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if cfg.CORSMaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.CORSMaxAge))
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
