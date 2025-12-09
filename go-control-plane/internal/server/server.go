package server

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/api"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/config"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/store"
	_ "github.com/lib/pq"
)

// Service wires config, backends, and HTTP server.
type Service struct {
	cfg config.Config
	mux *http.ServeMux
}

// New constructs the service with default backends.
func New(cfg config.Config) *Service {
	mux := http.NewServeMux()
	s := &Service{cfg: cfg, mux: mux}
	s.routes()
	return s
}

func (s *Service) routes() {
	db, err := sql.Open("postgres", s.cfg.PostgresDSN)
	if err != nil {
		log.Printf("warning: failed to open postgres: %v", err)
	}
	var st store.Store = store.NewPostgres(db)
	var q queue.Backend = queue.NewFileQueue(s.cfg.QueueFile)
	h := &api.Handler{Store: st, Queue: q}
	h.Routes(s.mux)
}

// Start runs the HTTP server.
func (s *Service) Start() error {
	log.Printf("starting server on %s", s.cfg.HTTPAddr)
	return http.ListenAndServe(s.cfg.HTTPAddr, s.mux)
}
