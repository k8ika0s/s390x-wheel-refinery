package main

import (
	"log"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/service"
)

func main() {
	if err := service.Run(); err != nil {
		log.Fatalf("worker exited: %v", err)
	}
}
