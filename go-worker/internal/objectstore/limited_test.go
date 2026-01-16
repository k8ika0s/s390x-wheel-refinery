package objectstore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type trackingStore struct {
	active int64
	max    int64
}

func (t *trackingStore) Put(_ context.Context, _ string, _ []byte, _ string) error {
	return t.withCall()
}

func (t *trackingStore) Get(_ context.Context, _ string) ([]byte, string, error) {
	if err := t.withCall(); err != nil {
		return nil, "", err
	}
	return nil, "", nil
}

func (t *trackingStore) URL(_ string) string { return "" }

func (t *trackingStore) withCall() error {
	cur := atomic.AddInt64(&t.active, 1)
	for {
		prev := atomic.LoadInt64(&t.max)
		if cur <= prev || atomic.CompareAndSwapInt64(&t.max, prev, cur) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	atomic.AddInt64(&t.active, -1)
	return nil
}

func TestLimitedStorePutRespectsConcurrency(t *testing.T) {
	store := &trackingStore{}
	limited := NewLimitedStore(store, 2)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = limited.Put(ctx, fmt.Sprintf("key-%d", i), []byte("x"), "application/octet-stream")
		}(i)
	}
	wg.Wait()
	if max := atomic.LoadInt64(&store.max); max > 2 {
		t.Fatalf("expected max concurrency <= 2, got %d", max)
	}
}

func TestLimitedStoreGetRespectsConcurrency(t *testing.T) {
	store := &trackingStore{}
	limited := NewLimitedStore(store, 3)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 7; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, _ = limited.Get(ctx, fmt.Sprintf("key-%d", i))
		}(i)
	}
	wg.Wait()
	if max := atomic.LoadInt64(&store.max); max > 3 {
		t.Fatalf("expected max concurrency <= 3, got %d", max)
	}
}
