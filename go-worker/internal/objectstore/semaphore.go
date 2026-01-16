package objectstore

import "golang.org/x/sync/semaphore"

type weightedSemaphore struct {
	*semaphore.Weighted
}

func newWeighted(max int) semaphore {
	if max <= 0 {
		return nil
	}
	return &weightedSemaphore{Weighted: semaphore.NewWeighted(int64(max))}
}
