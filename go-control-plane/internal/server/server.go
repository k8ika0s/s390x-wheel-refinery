package server

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/api"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/config"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/objectstore"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/queue"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/settings"
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
	if db != nil && !s.cfg.SkipMigrate {
		if err := store.RunMigrations(context.Background(), db); err != nil {
			log.Printf("warning: migration failed: %v", err)
		}
	}
	var st store.Store = store.NewPostgres(db)
	var q queue.Backend
	var planQ queue.PlanQueueBackend
	switch s.cfg.QueueBackend {
	case "redis":
		q = queue.NewRedisQueue(s.cfg.RedisURL, s.cfg.RedisKey)
		planQ = queue.NewPlanQueue(s.cfg.RedisURL, s.cfg.PlanRedisKey)
	case "kafka":
		q = queue.NewKafkaQueue(s.cfg.KafkaBrokers, s.cfg.KafkaTopic)
	default:
		q = queue.NewFileQueue(s.cfg.QueueFile)
	}
	// Load persisted settings to align auto-plan/build toggles on startup.
	if cfgFile := s.cfg.SettingsPath; cfgFile != "" {
		current := settings.Load(cfgFile)
		s.cfg.AutoPlan = settings.BoolValue(current.AutoPlan)
		s.cfg.AutoBuild = settings.BoolValue(current.AutoBuild)
	}
	if s.cfg.SeedHints {
		if res, err := store.SeedHintsFromDir(context.Background(), st, s.cfg.HintsDir); err != nil {
			log.Printf("warning: hint seed failed: %v", err)
		} else if res.Files > 0 {
			log.Printf("hint seed: files=%d loaded=%d skipped=%d errors=%d", res.Files, res.Loaded, res.Skipped, len(res.Errors))
		}
	}
	var inputStore objectstore.Store = objectstore.NullStore{}
	if s.cfg.ObjectStoreEndpoint != "" && s.cfg.ObjectStoreBucket != "" {
		if storeClient, err := objectstore.NewMinIOStore(
			s.cfg.ObjectStoreEndpoint,
			s.cfg.ObjectStoreAccess,
			s.cfg.ObjectStoreSecret,
			s.cfg.ObjectStoreBucket,
			s.cfg.ObjectStoreUseSSL,
		); err != nil {
			log.Printf("warning: input object store init failed: %v", err)
		} else {
			inputStore = storeClient
		}
	}
	h := &api.Handler{Store: st, Queue: q, PlanQ: planQ, Config: s.cfg, InputStore: inputStore}
	h.Routes(s.mux)
}

// Start runs the HTTP server.
func (s *Service) Start() error {
	log.Printf("starting server on %s", s.cfg.HTTPAddr)
	return http.ListenAndServe(s.cfg.HTTPAddr, withCORS(s.cfg, withGzip(s.mux)))
}
