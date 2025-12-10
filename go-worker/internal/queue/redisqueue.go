package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisQueue is a simple Redis-backed implementation using a list.
type RedisQueue struct {
	client *redis.Client
	key    string
}

// NewRedisQueue creates a Redis-backed queue. If url is empty, operations will error.
func NewRedisQueue(url, key string) *RedisQueue {
	if key == "" {
		key = "refinery:queue"
	}
	if url == "" {
		return &RedisQueue{client: nil, key: key}
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return &RedisQueue{client: nil, key: key}
	}
	return &RedisQueue{client: redis.NewClient(opt), key: key}
}

func (r *RedisQueue) ensure() error {
	if r.client == nil {
		return errors.New("redis queue not configured")
	}
	return nil
}

func (r *RedisQueue) Enqueue(ctx context.Context, req Request) error {
	if err := r.ensure(); err != nil {
		return err
	}
	if req.EnqueuedAt == 0 {
		req.EnqueuedAt = time.Now().Unix()
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return r.client.RPush(ctx, r.key, data).Err()
}

func (r *RedisQueue) List(ctx context.Context) ([]Request, error) {
	if err := r.ensure(); err != nil {
		return nil, err
	}
	vals, err := r.client.LRange(ctx, r.key, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	items := make([]Request, 0, len(vals))
	for _, v := range vals {
		var req Request
		if err := json.Unmarshal([]byte(v), &req); err == nil {
			items = append(items, req)
		}
	}
	return items, nil
}

func (r *RedisQueue) Clear(ctx context.Context) error {
	if err := r.ensure(); err != nil {
		return err
	}
	return r.client.Del(ctx, r.key).Err()
}

func (r *RedisQueue) Stats(ctx context.Context) (Stats, error) {
	if err := r.ensure(); err != nil {
		return Stats{}, err
	}
	lenCmd := r.client.LLen(ctx, r.key)
	length, err := lenCmd.Result()
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{Length: int(length)}
	if length > 0 {
		first, err := r.client.LIndex(ctx, r.key, 0).Result()
		if err == nil {
			var req Request
			if err := json.Unmarshal([]byte(first), &req); err == nil && req.EnqueuedAt > 0 {
				stats.OldestAge = time.Now().Unix() - req.EnqueuedAt
			}
		}
	}
	return stats, nil
}

func (r *RedisQueue) Pop(ctx context.Context, max int) ([]Request, error) {
	if err := r.ensure(); err != nil {
		return nil, err
	}
	items := []Request{}
	if max <= 0 {
		max = 1
	}
	for i := 0; i < max; i++ {
		val, err := r.client.LPop(ctx, r.key).Result()
		if errors.Is(err, redis.Nil) {
			break
		}
		if err != nil {
			return items, err
		}
		var req Request
		if err := json.Unmarshal([]byte(val), &req); err == nil {
			items = append(items, req)
		}
	}
	return items, nil
}
