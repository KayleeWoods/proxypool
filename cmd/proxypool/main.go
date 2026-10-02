package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/KayleeWoods/proxypool/internal/api"
	"github.com/KayleeWoods/proxypool/internal/config"
	"github.com/KayleeWoods/proxypool/internal/health"
	"github.com/KayleeWoods/proxypool/internal/pool"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "get":
		cmdGet(os.Args[2:])
	case "health":
		cmdHealth(os.Args[2:])
	case "stats":
		cmdStats(os.Args[2:])
	case "version":
		fmt.Printf("proxypool %s\n", version)
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`proxypool — proxy pool manager with health checks and rotation

Usage:
  proxypool serve   [-config path]     Start the HTTP API server + health loop
  proxypool get     [-config path] [-pool name] [-session id]
  proxypool health  [-config path]     Run a one-shot health check and print results
  proxypool stats   [-config path]     Print pool statistics
  proxypool version                     Print version

Flags:
  -config path   Config file (default: configs/example.yaml)
  -pool name     Pool name (default: first available)
  -session id    Session ID for sticky rotation`)
}

func loadConfig(args []string) *config.Config {
	path := "configs/example.yaml"
	for i, a := range args {
		if a == "-config" && i+1 < len(args) {
			path = args[i+1]
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func cmdServe(args []string) {
	cfg := loadConfig(args)
	mgr := pool.NewManager(cfg)
	checker := health.NewChecker(cfg, mgr)
	checker.Start()

	srv := api.NewServer(mgr)
	fmt.Printf("proxypool %s listening on %s\n", version, cfg.Server.Listen)
	if err := http.ListenAndServe(cfg.Server.Listen, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func cmdGet(args []string) {
	cfg := loadConfig(args)
	poolName := ""
	session := ""
	for i, a := range args {
		if a == "-pool" && i+1 < len(args) {
			poolName = args[i+1]
		}
		if a == "-session" && i+1 < len(args) {
			session = args[i+1]
		}
	}
	mgr := pool.NewManager(cfg)
	ps, err := mgr.Get(poolName, session)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(ps.URL())
}

func cmdHealth(args []string) {
	cfg := loadConfig(args)
	mgr := pool.NewManager(cfg)
	checker := health.NewChecker(cfg, mgr)
	checker.CheckAll()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "POOL\tHOST:PORT\tHEALTHY\tLATENCY\tFAILURES")
	for _, name := range mgr.Order() {
		for _, ps := range mgr.Pools()[name].States() {
			status := "UP"
			if !ps.IsAvailable() {
				status = "DOWN"
			}
			fmt.Fprintf(w, "%s\t%s:%d\t%s\t%dms\t%d\n",
				name, ps.Proxy.Host, ps.Proxy.Port, status, ps.LatencyMs, ps.Failures)
		}
	}
	_ = w.Flush()
}

func cmdStats(args []string) {
	cfg := loadConfig(args)
	mgr := pool.NewManager(cfg)
	checker := health.NewChecker(cfg, mgr)
	checker.CheckAll()

	stats := make([]map[string]interface{}, 0)
	for _, name := range mgr.Order() {
		stats = append(stats, mgr.Pools()[name].Stats())
	}
	out, _ := json.MarshalIndent(map[string]interface{}{"pools": stats}, "", "  ")
	fmt.Println(string(out))
}
