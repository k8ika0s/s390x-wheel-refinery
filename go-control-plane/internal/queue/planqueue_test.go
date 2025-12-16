package queue

import (
	"context"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
)

func TestPlanQueueEnqueueAndPop(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	url := "redis://" + mr.Addr()
	q := NewPlanQueue(url, "test:plan")

	ctx := context.Background()
	if err := q.Enqueue(ctx, "10"); err != nil {
		t.Fatalf("enqueue 10: %v", err)
	}
	if err := q.Enqueue(ctx, "11"); err != nil {
		t.Fatalf("enqueue 11: %v", err)
	}

	items, err := q.Pop(ctx, 5)
	if err != nil {
		t.Fatalf("pop: %v", err)
	}
	if len(items) != 2 || items[0] != "10" || items[1] != "11" {
		t.Fatalf("unexpected pop order: %+v", items)
	}
	// empty afterwards
	items, err = q.Pop(ctx, 1)
	if err != nil {
		t.Fatalf("pop empty: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty pop, got %+v", items)
	}

	llen, err := q.Len(ctx)
	if err != nil {
		t.Fatalf("len: %v", err)
	}
	if llen != 0 {
		t.Fatalf("expected len 0, got %d", llen)
	}
}
