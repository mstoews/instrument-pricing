package auth

import (
	"sync"
	"time"
)

// Limiter counts failures per key in a fixed window. It is in-memory and per
// process: with several API instances each enforces its own budget.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string]window
}

type window struct {
	start time.Time
	count int
}

const maxLimiterKeys = 100000

func NewLimiter(max int, w time.Duration) *Limiter {
	return &Limiter{max: max, window: w, now: time.Now, hits: map[string]window{}}
}

// Blocked reports whether key has exhausted its failures in the current window.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.hits[key]
	return ok && l.now().Sub(w.start) < l.window && w.count >= l.max
}

func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) >= maxLimiterKeys {
		for k, w := range l.hits {
			if now.Sub(w.start) >= l.window {
				delete(l.hits, k)
			}
		}
		if len(l.hits) >= maxLimiterKeys {
			l.hits = map[string]window{} // under a flood, prefer bounded memory over history
		}
	}
	w := l.hits[key]
	if now.Sub(w.start) >= l.window {
		w = window{start: now}
	}
	w.count++
	l.hits[key] = w
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
