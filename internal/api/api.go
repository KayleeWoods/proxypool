package api

import (
	"encoding/json"
	"net/http"

	"github.com/lugaysu/proxypool/internal/pool"
)

// Server provides HTTP API endpoints for proxy pool management.
type Server struct {
	mgr *pool.Manager
}

func NewServer(mgr *pool.Manager) *Server {
	return &Server{mgr: mgr}
}

// Handler returns the HTTP mux with all routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /proxy", s.handleGetProxy)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /stats", s.handleStats)
	mux.HandleFunc("GET /proxies", s.handleListProxies)
	return mux
}

// GET /proxy?pool=<name>&session=<id>
// Returns a single proxy endpoint.
func (s *Server) handleGetProxy(w http.ResponseWriter, r *http.Request) {
	poolName := r.URL.Query().Get("pool")
	session := r.URL.Query().Get("session")

	ps, err := s.mgr.Get(poolName, session)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"proxy":   ps.URL(),
		"socks5":  ps.SocksURL(),
		"host":    ps.Proxy.Host,
		"port":    ps.Proxy.Port,
		"latency": ps.LatencyMs,
	})
}

// GET /health
// Returns health status of all pools.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	pools := s.mgr.Pools()
	result := make([]map[string]interface{}, 0, len(pools))
	for _, name := range s.mgr.Order() {
		result = append(result, pools[name].Stats())
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"pools": result})
}

// GET /stats
// Returns aggregate statistics.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	pools := s.mgr.Pools()
	totalRequests := int64(0)
	totalHealthy := 0
	totalProxies := 0
	poolStats := make([]map[string]interface{}, 0, len(pools))
	for _, name := range s.mgr.Order() {
		stats := pools[name].Stats()
		poolStats = append(poolStats, stats)
		if v, ok := stats["requests"].(int64); ok {
			totalRequests += v
		}
		if v, ok := stats["healthy"].(int); ok {
			totalHealthy += v
		}
		if v, ok := stats["total"].(int); ok {
			totalProxies += v
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_proxies":  totalProxies,
		"total_healthy":  totalHealthy,
		"total_requests": totalRequests,
		"pools":          poolStats,
	})
}

// GET /proxies?pool=<name>
// Lists all proxies with their state.
func (s *Server) handleListProxies(w http.ResponseWriter, r *http.Request) {
	poolName := r.URL.Query().Get("pool")
	pools := s.mgr.Pools()
	result := make([]map[string]interface{}, 0)

	for _, name := range s.mgr.Order() {
		if poolName != "" && name != poolName {
			continue
		}
		for _, ps := range pools[name].States() {
			result = append(result, map[string]interface{}{
				"pool":              name,
				"host":              ps.Proxy.Host,
				"port":              ps.Proxy.Port,
				"healthy":           ps.IsAvailable(),
				"latency_ms":        ps.LatencyMs,
				"failures":          ps.Failures,
				"success_count":     ps.SuccessCount,
				"failure_count":     ps.FailureCount,
				"last_check":        ps.LastCheck,
				"quarantined_until": ps.QuarantinedUntil,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proxies": result})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
