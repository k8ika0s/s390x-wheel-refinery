package service

import (
	"context"
	"log"
	"net/http"
	"time"
)

// Run starts the worker HTTP server (stub for now).
func Run() error {
    cfg := fromEnv()
    w, err := BuildWorker(cfg)
    if err != nil {
        return err
    }
    mux := http.NewServeMux()
    mux.HandleFunc("/health", func(wr http.ResponseWriter, r *http.Request) {
        wr.WriteHeader(http.StatusOK)
        _, _ = wr.Write([]byte(`{"status":"ok"}`))
    })
    mux.HandleFunc("/ready", func(wr http.ResponseWriter, r *http.Request) {
        wr.WriteHeader(http.StatusOK)
        _, _ = wr.Write([]byte(`{"status":"ready"}`))
    })
    mux.HandleFunc("/trigger", func(wr http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            wr.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
        if cfg.WorkerToken != "" {
            tok := r.Header.Get("X-Worker-Token")
            if tok == "" {
                tok = r.URL.Query().Get("token")
            }
            if tok != cfg.WorkerToken {
                wr.WriteHeader(http.StatusForbidden)
                return
            }
        }
        ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
        defer cancel()
        if err := w.RunOnce(ctx); err != nil {
            wr.WriteHeader(http.StatusInternalServerError)
            _, _ = wr.Write([]byte(err.Error()))
            return
        }
        wr.WriteHeader(http.StatusOK)
        _, _ = wr.Write([]byte(`{"detail":"worker ran"}`))
    })
    srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
    go func() {
        <-context.Background().Done()
        _ = srv.Shutdown(context.Background())
    }()
    log.Printf("starting worker on %s", srv.Addr)
    return srv.ListenAndServe()
}
