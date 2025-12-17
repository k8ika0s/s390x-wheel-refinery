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
	Pop(ctx context.Context, max int) ([]string, error)
	Len(ctx context.Context) (int64, error)
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

// Pop pops up to max items (FIFO).
func (p *PlanQueue) Pop(ctx context.Context, max int) ([]string, error) {
	if err := p.ensure(); err != nil {
		return nil, err
	}
	if max <= 0 {
		max = 1
	}
	var out []string
	for i := 0; i < max; i++ {
		val, err := p.client.LPop(ctx, p.key).Result()
		if errors.Is(err, redis.Nil) {
			break
		}
		if err != nil {
			return out, err
		}
		var payload map[string]string
		if err := json.Unmarshal([]byte(val), &payload); err != nil {
			// push back malformed payload? For now, skip but continue.
			continue
		}
		if id, ok := payload["pending_input_id"]; ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// Len returns queue length.
func (p *PlanQueue) Len(ctx context.Context) (int64, error) {
	if err := p.ensure(); err != nil {
		return 0, err
	}
	return p.client.LLen(ctx, p.key).Val(), nil
}
