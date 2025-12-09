package server

import "net/http"

// Service is a thin wrapper around the HTTP server.
type Service struct {
	mux *http.ServeMux
}

// New constructs the service with placeholder routes.
func New() *Service {
	mux := http.NewServeMux()
	// Placeholder health route; real routes wired from handlers package later.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return &Service{mux: mux}
}

// Start runs the HTTP server on default port 8080.
func (s *Service) Start() error {
	return http.ListenAndServe(":8080", s.mux)
}
