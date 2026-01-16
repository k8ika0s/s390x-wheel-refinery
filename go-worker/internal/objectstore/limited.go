package objectstore

import "context"

// LimitedStore wraps a store with a concurrency limiter for Put/Get.
type LimitedStore struct {
	Store Store
	sem   semaphore
}

type semaphore interface {
	Acquire(context.Context, int64) error
	Release(int64)
}

// NewLimitedStore returns a Store that limits concurrent calls when max > 0.
func NewLimitedStore(store Store, max int) Store {
	if store == nil || max <= 0 {
		return store
	}
	return &LimitedStore{
		Store: store,
		sem:   newWeighted(max),
	}
}

func (l *LimitedStore) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if l.sem == nil {
		return l.Store.Put(ctx, key, data, contentType)
	}
	if err := l.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer l.sem.Release(1)
	return l.Store.Put(ctx, key, data, contentType)
}

func (l *LimitedStore) Get(ctx context.Context, key string) ([]byte, string, error) {
	if l.sem == nil {
		return l.Store.Get(ctx, key)
	}
	if err := l.sem.Acquire(ctx, 1); err != nil {
		return nil, "", err
	}
	defer l.sem.Release(1)
	return l.Store.Get(ctx, key)
}

func (l *LimitedStore) URL(key string) string {
	return l.Store.URL(key)
}
