package health

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/lugaysu/proxypool/internal/config"
	"github.com/lugaysu/proxypool/internal/pool"
	"golang.org/x/net/proxy"
)

// Checker runs periodic health checks against all proxies.
type Checker struct {
	cfg  *config.Config
	mgr  *pool.Manager
	stop chan struct{}
	wg   sync.WaitGroup
}

func NewChecker(cfg *config.Config, mgr *pool.Manager) *Checker {
	return &Checker{
		cfg:  cfg,
		mgr:  mgr,
		stop: make(chan struct{}),
	}
}

// Start begins the periodic health check loop.
func (c *Checker) Start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.cfg.Health.Interval)
		defer ticker.Stop()
		// run immediately on start
		c.CheckAll()
		for {
			select {
			case <-ticker.C:
				c.CheckAll()
				// cleanup expired sticky sessions
				for _, p := range c.mgr.Pools() {
					p.CleanupSticky()
				}
			case <-c.stop:
				return
			}
		}
	}()
}

// Stop halts the health check loop.
func (c *Checker) Stop() {
	close(c.stop)
	c.wg.Wait()
}

// CheckAll runs a health check on every proxy in every pool.
func (c *Checker) CheckAll() {
	var wg sync.WaitGroup
	for _, p := range c.mgr.Pools() {
		states := p.States()
		for i, ps := range states {
			wg.Add(1)
			go func(idx int, st *pool.ProxyState) {
				defer wg.Done()
				healthy, latency := c.checkProxy(st)
				p.SetHealth(idx, healthy, latency, c.cfg.Health.MaxFailures, c.cfg.Health.QuarantineDuration)
			}(i, ps)
		}
	}
	wg.Wait()
}

// checkProxy tests a single proxy by making an HTTP request through it.
func (c *Checker) checkProxy(ps *pool.ProxyState) (bool, int64) {
	start := time.Now()
	client, err := c.buildClient(ps)
	if err != nil {
		return false, 0
	}
	resp, err := client.Get(c.cfg.Health.TestURL)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return false, latency
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == 200, latency
}

func (c *Checker) buildClient(ps *pool.ProxyState) (*http.Client, error) {
	timeout := c.cfg.Health.Timeout
	proxyType := "http"

	// determine proxy type from pool
	for _, p := range c.mgr.Pools() {
		for _, s := range p.States() {
			if s.Proxy == ps.Proxy {
				proxyType = p.Type()
				break
			}
		}
	}

	switch proxyType {
	case "socks5":
		var auth *proxy.Auth
		if ps.Proxy.Username != "" {
			auth = &proxy.Auth{
				User:     ps.Proxy.Username,
				Password: ps.Proxy.Password,
			}
		}
		dialer, err := proxy.SOCKS5("tcp", ps.Proxy.Addr(), auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer: %w", err)
		}
		return &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Dial: dialer.Dial,
			},
		}, nil

	default: // http
		proxyURL, err := url.Parse(ps.URL())
		if err != nil {
			return nil, fmt.Errorf("parse proxy url: %w", err)
		}
		return &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}, nil
	}
}
