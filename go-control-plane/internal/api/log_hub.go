package api

import (
	"strings"
	"sync"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/store"
)

type logHub struct {
	mu   sync.RWMutex
	subs map[string]map[chan store.LogChunk]struct{}
}

func newLogHub() *logHub {
	return &logHub{subs: make(map[string]map[chan store.LogChunk]struct{})}
}

func (h *Handler) getLogHub() *logHub {
	h.logHubOnce.Do(func() {
		h.logHub = newLogHub()
	})
	return h.logHub
}

func (h *logHub) subscribe(key string) (chan store.LogChunk, func()) {
	ch := make(chan store.LogChunk, 256)
	h.mu.Lock()
	if h.subs[key] == nil {
		h.subs[key] = make(map[chan store.LogChunk]struct{})
	}
	h.subs[key][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if subs, ok := h.subs[key]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(h.subs, key)
			}
		}
		h.mu.Unlock()
		close(ch)
	}
}

func (h *logHub) publish(key string, chunk store.LogChunk) {
	h.mu.RLock()
	subs := h.subs[key]
	for ch := range subs {
		select {
		case ch <- chunk:
		default:
		}
	}
	h.mu.RUnlock()
}

func logStreamKey(name, version string) string {
	return strings.ToLower(name) + "::" + strings.ToLower(version)
}
