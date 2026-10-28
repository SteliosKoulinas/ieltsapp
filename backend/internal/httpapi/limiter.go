package httpapi

import (
	"sync"
	"time"
)

// limiter is a small token bucket. It caps how many evaluations this server
// starts per minute, so a loop or a double click cannot burn the provider's
// free quota. It is local to one process, which is enough for this app.
type limiter struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	perSec   float64
	last     time.Time
	now      func() time.Time
}

func newLimiter(perMinute int, now func() time.Time) *limiter {
	capacity := float64(max(3, perMinute/2))
	if capacity > float64(perMinute) {
		capacity = float64(perMinute)
	}
	return &limiter{tokens: capacity, capacity: capacity, perSec: float64(perMinute) / 60, last: now(), now: now}
}

func (l *limiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	l.tokens = min(l.capacity, l.tokens+t.Sub(l.last).Seconds()*l.perSec)
	l.last = t
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
