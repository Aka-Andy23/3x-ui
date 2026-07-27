package sub

import (
	"sync"
	"time"
)

type requestLimitBucket struct {
	start time.Time
	count int
}

type requestLimiter struct {
	mu      sync.Mutex
	buckets map[string]requestLimitBucket
	limit   int
	window  time.Duration
	maxKeys int
	now     func() time.Time
}

func newRequestLimiter(limit int, window time.Duration) *requestLimiter {
	return &requestLimiter{
		buckets: map[string]requestLimitBucket{},
		limit:   limit,
		window:  window,
		maxKeys: 4096,
		now:     time.Now,
	}
}

func (l *requestLimiter) allow(key string) (time.Duration, bool) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.buckets[key]; !exists && len(l.buckets) >= l.maxKeys {
		l.prune(now)
		if len(l.buckets) >= l.maxKeys {
			key = "*"
		}
	}
	bucket := l.buckets[key]
	if bucket.start.IsZero() || now.Sub(bucket.start) >= l.window {
		l.buckets[key] = requestLimitBucket{start: now, count: 1}
		l.prune(now)
		return 0, true
	}
	if bucket.count >= l.limit {
		return max(time.Second, l.window-now.Sub(bucket.start)), false
	}
	bucket.count++
	l.buckets[key] = bucket
	return 0, true
}

func (l *requestLimiter) prune(now time.Time) {
	for key, bucket := range l.buckets {
		if now.Sub(bucket.start) >= l.window {
			delete(l.buckets, key)
		}
	}
}
