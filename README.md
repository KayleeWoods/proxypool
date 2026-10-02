# proxypool

Proxy pool manager with health checks, rotation strategies, and a local HTTP API.

Manage proxies from multiple providers (Bright Data, WARP, residential, datacenter) in one place. Automatic health checking, failover, sticky sessions, and a single local endpoint that routes to the best available proxy.

## Features

- **Multi-provider registry** — add proxies from any source (HTTP, SOCKS5) in one YAML config
- **Auto health check** — periodic latency + availability checks, auto-quarantine dead proxies
- **Rotation strategies** — round-robin, random, or sticky (TTL-based session affinity)
- **Local proxy endpoint** — `GET /proxy` returns a ready-to-use proxy URL
- **HTTP API** — query pool health, stats, and proxy lists from any application
- **CLI** — quick proxy grab, health table, and stats from the terminal

## Quick Start

```bash
# Clone and build
git clone https://github.com/lugaysu/proxypool.git
cd proxypool
make build

# Copy and edit config
cp configs/example.yaml configs/my.yaml
vim configs/my.yaml

# Start server (health loop + HTTP API)
./bin/proxypool serve -config configs/my.yaml

# Get a proxy
./bin/proxypool get -config configs/my.yaml

# Get a sticky proxy (same proxy per session)
./bin/proxypool get -config configs/my.yaml -session user123

# Health check table
./bin/proxypool health -config configs/my.yaml

# Pool stats
./bin/proxypool stats -config configs/my.yaml
```

## HTTP API

The server exposes four endpoints (default `127.0.0.1:8899`):

| Endpoint | Description |
|----------|-------------|
| `GET /proxy?pool=<name>&session=<id>` | Get a proxy URL (auto-rotated) |
| `GET /health` | Health status of all pools |
| `GET /stats` | Aggregate statistics |
| `GET /proxies?pool=<name>` | List all proxies with state |

### Examples

```bash
# Get a proxy from the "warp" pool
curl "http://127.0.0.1:8899/proxy?pool=warp"

# Get a sticky proxy (same proxy for session "user123")
curl "http://127.0.0.1:8899/proxy?pool=brightdata&session=user123"

# Check overall health
curl "http://127.0.0.1:8899/health"

# List all proxies with state
curl "http://127.0.0.1:8899/proxies"
```

## Configuration

```yaml
server:
  listen: "127.0.0.1:8899"

health:
  test_url: "http://api.ipify.org?format=json"
  interval: 60s          # check every 60s
  timeout: 10s           # per-request timeout
  max_failures: 3        # quarantine after 3 consecutive failures
  quarantine_duration: 300s  # auto-recover after 5 minutes

pools:
  - name: brightdata
    type: http           # http or socks5
    rotation: sticky     # round_robin, random, or sticky
    sticky_ttl: 300s     # session affinity duration
    proxies:
      - host: brd.superproxy.io
        port: 22225
        username: hl_c216816a
        password: YOUR_PASSWORD

  - name: warp
    type: socks5
    rotation: round_robin
    proxies:
      - host: 127.0.0.1
        port: 40000
```

## Integration

Use `proxypool` as a proxy source in any tool:

```bash
# curl through the pool
PROXY=$(curl -s "http://127.0.0.1:8899/proxy" | jq -r .proxy)
curl -x "$PROXY" https://httpbin.org/ip

# Playwright / Puppeteer
proxy = await fetch('http://127.0.0.1:8899/proxy').then(r => r.json())
browser = await chromium.launch({ proxy: { server: proxy.proxy } })

# Python requests
import requests
proxy = requests.get('http://127.0.0.1:8899/proxy').json()
requests.get('https://httpbin.org/ip', proxies={'http': proxy['proxy'], 'https': proxy['proxy']})
```

## License

MIT
