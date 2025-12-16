package queue

import (
	"context"
	"encoding/json"
	"errors"

	redis "github.com/redis/go-redis/v9"
)

// PlanQueueBackend pushes/pops plan generation requests (by ID).
type PlanQueueBackend interface {
	Enqueue(ctx context.Context, id string) error
}

// PlanQueue is a Redis-backed queue for plan IDs.
type PlanQueue struct {
	client *redis.Client
	key    string
}

func NewPlanQueue(url, key string) *PlanQueue {
	if key == "" {
		key = "refinery:plan_queue"
	}
	if url == "" {
		return &PlanQueue{client: nil, key: key}
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return &PlanQueue{client: nil, key: key}
	}
	return &PlanQueue{client: redis.NewClient(opt), key: key}
}

func (p *PlanQueue) ensure() error {
	if p.client == nil {
		return errors.New("plan queue not configured")
	}
	return nil
}

func (p *PlanQueue) Enqueue(ctx context.Context, id string) error {
	if err := p.ensure(); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"pending_input_id": id})
	return p.client.RPush(ctx, p.key, payload).Err()
}
