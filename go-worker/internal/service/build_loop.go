package service

import (
	"context"
	"log"
	"time"
)

func buildLoop(ctx context.Context, cfg Config, runDrain func(context.Context) (bool, error)) {
	interval := time.Duration(cfg.BuildPollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		ran, err := runDrain(runCtx)
		cancel()
		if err != nil {
			log.Printf("build loop: %v", err)
		} else if !ran {
			log.Printf("build loop: skip (drain already running)")
		}
		timer.Reset(interval)
	}
}
