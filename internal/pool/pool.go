package pool

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lugaysu/proxypool/internal/config"
)

// ProxyState tracks runtime health and stats for a single proxy.
type ProxyState struct {
	Proxy            config.ProxyConfig `json:"proxy"`
	Healthy          bool               `json:"healthy"`
	Failures         int                `json:"failures"`
	LastCheck        time.Time          `json:"last_check"`
	QuarantinedUntil time.Time          `json:"quarantined_until"`
	LatencyMs        int64              `json:"latency_ms"`
	SuccessCount     int64              `json:"success_count"`
	FailureCount     int64              `json:"failure_count"`
}

func (ps *ProxyState) IsAvailable() bool {
	if ps.QuarantinedUntil.After(time.Now()) {
		return false
	}
	return ps.Healthy
}

func (ps *ProxyState) URL() string {
	return ps.Proxy.URL("http")
}

func (ps *ProxyState) SocksURL() string {
	return ps.Proxy.URL("socks5")
}

// stickyEntry holds a session-to-proxy binding.
type stickyEntry struct {
	proxyIdx int
	expires  time.Time
}

// Pool manages a named set of proxies with rotation and health state.
type Pool struct {
	mu        sync.RWMutex
	name      string
	poolType  string
	rotation  string
	stickyTTL time.Duration
	proxies   []*ProxyState
	rrIndex   uint64
	sticky    map[string]*stickyEntry
	requests  int64
}

func NewPool(cfg config.PoolConfig) *Pool {
	p := &Pool{
		name:      cfg.Name,
		poolType:  cfg.Type,
		rotation:  cfg.Rotation,
		stickyTTL: cfg.StickyTTL,
		sticky:    make(map[string]*stickyEntry),
	}
	for _, pc := range cfg.Proxies {
		p.proxies = append(p.proxies, &ProxyState{
			Proxy:   pc,
			Healthy: true, // assume healthy until first check
		})
	}
	return p
}

func (p *Pool) Name() string     { return p.name }
func (p *Pool) Type() string     { return p.poolType }
func (p *Pool) Rotation() string { return p.rotation }
func (p *Pool) Count() int       { return len(p.proxies) }

func (p *Pool) healthyIndices() []int {
	var out []int
	for i, ps := range p.proxies {
		if ps.IsAvailable() {
			out = append(out, i)
		}
	}
	return out
}

// Get returns the next available proxy based on the rotation strategy.
// session is used for sticky rotation (ignored for other strategies).
func (p *Pool) Get(session string) (*ProxyState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	atomic.AddInt64(&p.requests, 1)

	healthy := p.healthyIndices()
	if len(healthy) == 0 {
		return nil, fmt.Errorf("pool %q: no healthy proxies", p.name)
	}

	// Sticky: return bound proxy if still valid
	if p.rotation == "sticky" && session != "" {
		if entry, ok := p.sticky[session]; ok {
			if time.Now().Before(entry.expires) && p.proxies[entry.proxyIdx].IsAvailable() {
				entry.expires = time.Now().Add(p.stickyTTL) // refresh TTL
				return p.proxies[entry.proxyIdx], nil
			}
			delete(p.sticky, session)
		}
	}

	var idx int
	switch p.rotation {
	case "random":
		idx = healthy[rand.Intn(len(healthy))]
	case "sticky":
		idx = healthy[rand.Intn(len(healthy))]
		if session != "" {
			p.sticky[session] = &stickyEntry{
				proxyIdx: idx,
				expires:  time.Now().Add(p.stickyTTL),
			}
		}
	default: // round_robin
		n := atomic.AddUint64(&p.rrIndex, 1)
		idx = healthy[int(n)%len(healthy)]
	}
	return p.proxies[idx], nil
}

// MarkSuccess records a successful request through the proxy.
func (p *Pool) MarkSuccess(idx int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if idx >= 0 && idx < len(p.proxies) {
		p.proxies[idx].SuccessCount++
		p.proxies[idx].Failures = 0
		p.proxies[idx].Healthy = true
	}
}

// MarkFailure records a failed request and quarantines if needed.
func (p *Pool) MarkFailure(idx int, maxFailures int, quarantineDuration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if idx >= 0 && idx < len(p.proxies) {
		p.proxies[idx].FailureCount++
		p.proxies[idx].Failures++
		if p.proxies[idx].Failures >= maxFailures {
			p.proxies[idx].Healthy = false
			p.proxies[idx].QuarantinedUntil = time.Now().Add(quarantineDuration)
		}
	}
}

// SetHealth updates health status after a health check.
func (p *Pool) SetHealth(idx int, healthy bool, latencyMs int64, maxFailures int, quarantineDuration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if idx < 0 || idx >= len(p.proxies) {
		return
	}
	ps := p.proxies[idx]
	ps.LastCheck = time.Now()
	ps.LatencyMs = latencyMs
	if healthy {
		ps.Healthy = true
		ps.Failures = 0
		ps.SuccessCount++
		ps.QuarantinedUntil = time.Time{}
	} else {
		ps.FailureCount++
		ps.Failures++
		if ps.Failures >= maxFailures {
			ps.Healthy = false
			ps.QuarantinedUntil = time.Now().Add(quarantineDuration)
		}
	}
}

// States returns a snapshot of all proxy states.
func (p *Pool) States() []*ProxyState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*ProxyState, len(p.proxies))
	copy(out, p.proxies)
	return out
}

// Stats returns aggregate stats.
func (p *Pool) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	healthy := 0
	quarantined := 0
	for _, ps := range p.proxies {
		if ps.QuarantinedUntil.After(time.Now()) {
			quarantined++
		} else if ps.Healthy {
			healthy++
		}
	}
	return map[string]interface{}{
		"name":        p.name,
		"type":        p.poolType,
		"rotation":    p.rotation,
		"total":       len(p.proxies),
		"healthy":     healthy,
		"quarantined": quarantined,
		"requests":    atomic.LoadInt64(&p.requests),
	}
}

// CleanupSticky removes expired sticky session bindings.
func (p *Pool) CleanupSticky() {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for k, v := range p.sticky {
		if now.After(v.expires) {
			delete(p.sticky, k)
		}
	}
}

// Manager holds all pools.
type Manager struct {
	pools map[string]*Pool
	order []string
}

func NewManager(cfg *config.Config) *Manager {
	m := &Manager{pools: make(map[string]*Pool)}
	for _, pc := range cfg.Pools {
		p := NewPool(pc)
		m.pools[pc.Name] = p
		m.order = append(m.order, pc.Name)
	}
	return m
}

func (m *Manager) Get(poolName, session string) (*ProxyState, error) {
	if poolName == "" {
		// return from first pool with available proxies
		for _, name := range m.order {
			p := m.pools[name]
			if ps, err := p.Get(session); err == nil {
				return ps, nil
			}
		}
		return nil, fmt.Errorf("no healthy proxies in any pool")
	}
	p, ok := m.pools[poolName]
	if !ok {
		return nil, fmt.Errorf("pool %q not found", poolName)
	}
	return p.Get(session)
}

func (m *Manager) Pools() map[string]*Pool {
	return m.pools
}

func (m *Manager) Order() []string {
	return m.order
}
