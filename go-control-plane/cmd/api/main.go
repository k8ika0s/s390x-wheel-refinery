package main

import (
	"log"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/config"
	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/server"
)

func main() {
	cfg := config.FromEnv()
	svc := server.New(cfg)
	if err := svc.Start(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
