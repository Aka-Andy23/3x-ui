package sub

import (
	"testing"
	"time"
)

func TestRequestLimiterBlocksAndResets(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := newRequestLimiter(2, time.Minute)
	limiter.now = func() time.Time { return now }
	if _, ok := limiter.allow("192.0.2.1"); !ok {
		t.Fatal("first request blocked")
	}
	if _, ok := limiter.allow("192.0.2.1"); !ok {
		t.Fatal("second request blocked")
	}
	if retry, ok := limiter.allow("192.0.2.1"); ok || retry <= 0 {
		t.Fatalf("limit not enforced: ok=%v retry=%v", ok, retry)
	}
	if _, ok := limiter.allow("192.0.2.2"); !ok {
		t.Fatal("separate address blocked")
	}
	now = now.Add(time.Minute)
	if _, ok := limiter.allow("192.0.2.1"); !ok {
		t.Fatal("window did not reset")
	}
}

func TestRequestLimiterBoundsUniqueKeys(t *testing.T) {
	limiter := newRequestLimiter(2, time.Minute)
	limiter.maxKeys = 2
	if _, ok := limiter.allow("a"); !ok {
		t.Fatal("first key blocked")
	}
	if _, ok := limiter.allow("b"); !ok {
		t.Fatal("second key blocked")
	}
	if _, ok := limiter.allow("c"); !ok {
		t.Fatal("first overflow request blocked")
	}
	if _, ok := limiter.allow("d"); !ok {
		t.Fatal("second overflow request blocked")
	}
	if _, ok := limiter.allow("e"); ok {
		t.Fatal("overflow bucket did not enforce the limit")
	}
	if len(limiter.buckets) != 3 {
		t.Fatalf("bucket count=%d, want 3 including overflow", len(limiter.buckets))
	}
}
