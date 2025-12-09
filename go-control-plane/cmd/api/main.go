package main

import (
	"log"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/server"
)

func main() {
	svc := server.New()
	if err := svc.Start(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
