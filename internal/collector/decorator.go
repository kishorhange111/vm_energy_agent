package collector

import (
	"errors"
	"sync"
	"time"
)

// ErrStaleMetrics is returned by CachedSource when data has exceeded its TTL
// and the underlying Collect() failed. Callers should treat this as "no fresh data".
var ErrStaleMetrics = errors.New("metrics are stale")

// CachedSource is a Decorator that wraps any MetricSource.
// It reduces CPU usage by caching results and avoids returning very old data.
type CachedSource struct {
	mu         sync.Mutex
	inner      MetricSource
	ttl        time.Duration
	lastResult *Metrics
	lastTime   time.Time
}

func NewCachedSource(inner MetricSource, ttl time.Duration) *CachedSource {
	return &CachedSource{inner: inner, ttl: ttl}
}

// Inner returns the wrapped source (useful for type inspection in visitors).
func (c *CachedSource) Inner() MetricSource {
	return c.inner
}

func (c *CachedSource) Name() string {
	return c.inner.Name()
}

func (c *CachedSource) Children() []MetricSource {
	return c.inner.Children()
}

func (c *CachedSource) Accept(v Visitor) {
	v.Visit(c) // pass self so visitor calls Collect() on the *CachedSource* → caching works
}

func (c *CachedSource) Collect() (*Metrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// 1. Return fresh cached data if still valid
	if c.lastResult != nil && now.Sub(c.lastTime) < c.ttl {
		return c.lastResult, nil
	}

	// 2. Try to get fresh data
	m, err := c.inner.Collect()
	if err == nil {
		c.lastResult = m
		c.lastTime = now
		return m, nil
	}

	// 3. Collection failed and we have no (or too old) cached data
	if c.lastResult != nil {
		// Data exists but is older than TTL → signal staleness instead of
		// polluting Prometheus with sentinel -1 values.
		return nil, ErrStaleMetrics
	}

	// 4. No previous data + collection failed
	return nil, err
}