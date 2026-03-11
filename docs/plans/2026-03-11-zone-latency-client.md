# Zone Latency Client Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go client that sends HTTP traffic between Kubernetes zones and exposes latency metrics for Prometheus.

**Architecture:** Single `main.go` binary with a ticker-based traffic sender goroutine and a metrics HTTP server. Configured entirely via environment variables. Deployed as 9 Kubernetes deployments covering all zone-to-zone combinations.

**Tech Stack:** Go, `prometheus/client_golang`, `promhttp`, Docker multi-stage build, Kubernetes manifests

**Spec:** `docs/superpowers/specs/2026-03-11-zone-latency-client-design.md`

---

## Chunk 1: Go Binary

### Task 1: Initialize Go module

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
cd /Users/zufardhiyaulhaq/Documents/personal/github/kubernetes-zone-latency-exporter
go mod init github.com/zufardhiyaulhaq/kubernetes-zone-latency-exporter
```

- [ ] **Step 2: Add prometheus dependency**

Run:
```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: initialize Go module with prometheus dependency"
```

---

### Task 2: Implement main.go — config loading

**Files:**
- Create: `main.go`

- [ ] **Step 1: Write main.go with config struct and loading**

```go
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

type Config struct {
	SourceZone        string
	DestinationZone   string
	DestinationURL    string
	RequestPath       string
	RequestMethod     string
	RequestsPerSecond int
	MetricsPort       int
	RequestTimeout    time.Duration
}

func loadConfig() (Config, error) {
	cfg := Config{
		RequestPath:       getEnvOrDefault("REQUEST_PATH", "/"),
		RequestMethod:     getEnvOrDefault("REQUEST_METHOD", "GET"),
		RequestsPerSecond: 10,
		MetricsPort:       9090,
		RequestTimeout:    5 * time.Second,
	}

	cfg.SourceZone = os.Getenv("SOURCE_ZONE")
	if cfg.SourceZone == "" {
		return cfg, fmt.Errorf("SOURCE_ZONE is required")
	}

	cfg.DestinationZone = os.Getenv("DESTINATION_ZONE")
	if cfg.DestinationZone == "" {
		return cfg, fmt.Errorf("DESTINATION_ZONE is required")
	}

	cfg.DestinationURL = os.Getenv("DESTINATION_URL")
	if cfg.DestinationURL == "" {
		return cfg, fmt.Errorf("DESTINATION_URL is required")
	}

	if v := os.Getenv("REQUESTS_PER_SECOND"); v != "" {
		rps, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid REQUESTS_PER_SECOND: %w", err)
		}
		cfg.RequestsPerSecond = rps
	}

	if v := os.Getenv("METRICS_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid METRICS_PORT: %w", err)
		}
		cfg.MetricsPort = port
	}

	if v := os.Getenv("REQUEST_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid REQUEST_TIMEOUT: %w", err)
		}
		cfg.RequestTimeout = d
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("starting zone-latency-client: %s -> %s (%s%s) at %d req/s",
		cfg.SourceZone, cfg.DestinationZone, cfg.DestinationURL, cfg.RequestPath, cfg.RequestsPerSecond)
}
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build -o /dev/null .
```
Expected: exits 0, no errors.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: add config loading from environment variables"
```

---

### Task 3: Implement metrics registration

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add Prometheus metric variables and registration**

Add these imports to the import block:
```go
"github.com/prometheus/client_golang/prometheus"
"github.com/prometheus/client_golang/prometheus/promauto"
```

Add after the `getEnvOrDefault` function:

```go
var (
	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "zone_latency_request_duration_seconds",
		Help:    "Duration of HTTP requests between zones",
		Buckets: prometheus.DefBuckets,
	}, []string{"source_zone", "destination_zone", "method", "path", "status_code"})

	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "zone_latency_requests_total",
		Help: "Total number of HTTP requests between zones",
	}, []string{"source_zone", "destination_zone", "method", "path", "status_code"})

	requestErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "zone_latency_request_errors_total",
		Help: "Total number of failed HTTP requests between zones",
	}, []string{"source_zone", "destination_zone", "method", "path"})

	requestsInFlight = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "zone_latency_requests_in_flight",
		Help: "Number of in-flight HTTP requests between zones",
	}, []string{"source_zone", "destination_zone"})
)
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
go build -o /dev/null .
```
Expected: exits 0, no errors.

- [ ] **Step 3: Commit**

```bash
git add main.go go.mod go.sum
git commit -m "feat: add Prometheus metric definitions"
```

---

### Task 4: Implement the traffic sender loop

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add sendRequest function and ticker loop**

Add these new imports to the import block (note: `"strconv"` and `"time"` already exist from Task 2, only add the new ones):
```go
"context"
"io"
"net/http"
```

Add after the metric variables:

```go
func sendRequest(ctx context.Context, client *http.Client, cfg Config) {
	requestsInFlight.WithLabelValues(cfg.SourceZone, cfg.DestinationZone).Inc()
	defer requestsInFlight.WithLabelValues(cfg.SourceZone, cfg.DestinationZone).Dec()

	url := cfg.DestinationURL + cfg.RequestPath
	req, err := http.NewRequestWithContext(ctx, cfg.RequestMethod, url, nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		requestErrors.WithLabelValues(cfg.SourceZone, cfg.DestinationZone, cfg.RequestMethod, cfg.RequestPath).Inc()
		return
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start).Seconds()

	if err != nil {
		log.Printf("request failed: %v", err)
		requestErrors.WithLabelValues(cfg.SourceZone, cfg.DestinationZone, cfg.RequestMethod, cfg.RequestPath).Inc()
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	statusCode := strconv.Itoa(resp.StatusCode)
	requestDuration.WithLabelValues(cfg.SourceZone, cfg.DestinationZone, cfg.RequestMethod, cfg.RequestPath, statusCode).Observe(duration)
	requestsTotal.WithLabelValues(cfg.SourceZone, cfg.DestinationZone, cfg.RequestMethod, cfg.RequestPath, statusCode).Inc()
}
```

- [ ] **Step 2: Update main() to start the ticker loop and metrics server**

Replace the existing `main()` function with:

```go
func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Printf("starting zone-latency-client: %s -> %s (%s%s) at %d req/s",
		cfg.SourceZone, cfg.DestinationZone, cfg.DestinationURL, cfg.RequestPath, cfg.RequestsPerSecond)

	client := &http.Client{
		Timeout: cfg.RequestTimeout,
	}

	// Start metrics server
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		addr := fmt.Sprintf(":%d", cfg.MetricsPort)
		log.Printf("serving metrics on %s/metrics", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("metrics server failed: %v", err)
		}
	}()

	// Start traffic sender loop
	interval := time.Second / time.Duration(cfg.RequestsPerSecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("sending requests every %s", interval)
	for range ticker.C {
		go sendRequest(context.Background(), client, cfg)
	}
}
```

Add to imports:
```go
"github.com/prometheus/client_golang/prometheus/promhttp"
```

- [ ] **Step 3: Verify it compiles**

Run:
```bash
go build -o /dev/null .
```
Expected: exits 0, no errors.

- [ ] **Step 4: Quick smoke test**

Run in one terminal:
```bash
SOURCE_ZONE=test-a DESTINATION_ZONE=test-b DESTINATION_URL=http://httpbin.org REQUEST_PATH=/get REQUESTS_PER_SECOND=1 go run .
```

In another terminal, verify metrics are served:
```bash
curl -s http://localhost:9090/metrics | grep zone_latency
```

Expected: should see `zone_latency_request_duration_seconds`, `zone_latency_requests_total`, etc. with labels `source_zone="test-a"`, `destination_zone="test-b"`.

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "feat: add traffic sender loop and metrics server"
```

---

### Task 5: Dockerfile

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Write Dockerfile**

```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o zone-latency-client .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/zone-latency-client /zone-latency-client
USER nonroot:nonroot
ENTRYPOINT ["/zone-latency-client"]
```

- [ ] **Step 2: Verify Docker build**

Run:
```bash
docker build -t zone-latency-client:latest .
```
Expected: builds successfully.

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "feat: add multi-stage Dockerfile"
```

---

## Chunk 2: Kubernetes Manifests

### Task 6: Client deployment manifests (9 files)

**Files:**
- Create: `client/deployment-5a-to-5a.yaml`
- Create: `client/deployment-5a-to-5b.yaml`
- Create: `client/deployment-5a-to-5c.yaml`
- Create: `client/deployment-5b-to-5a.yaml`
- Create: `client/deployment-5b-to-5b.yaml`
- Create: `client/deployment-5b-to-5c.yaml`
- Create: `client/deployment-5c-to-5a.yaml`
- Create: `client/deployment-5c-to-5b.yaml`
- Create: `client/deployment-5c-to-5c.yaml`

Each manifest follows this template (example for 5a → 5b):

- [ ] **Step 1: Create all 9 client deployment manifests**

Each file follows this pattern, substituting `{SRC}` and `{DST}`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zone-latency-client-{SRC}-to-{DST}
  namespace: infrastructure-engineering
  labels:
    app: zone-latency-client
    source-zone: ap-southeast-{SRC}
    destination-zone: ap-southeast-{DST}
spec:
  replicas: 2
  selector:
    matchLabels:
      app: zone-latency-client
      source-zone: ap-southeast-{SRC}
      destination-zone: ap-southeast-{DST}
  template:
    metadata:
      labels:
        app: zone-latency-client
        source-zone: ap-southeast-{SRC}
        destination-zone: ap-southeast-{DST}
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: topology.kubernetes.io/zone
                    operator: In
                    values:
                      - ap-southeast-{SRC}
      containers:
        - name: zone-latency-client
          image: zone-latency-client:latest
          ports:
            - containerPort: 9090
              name: metrics
          env:
            - name: SOURCE_ZONE
              value: "ap-southeast-{SRC}"
            - name: DESTINATION_ZONE
              value: "ap-southeast-{DST}"
            - name: DESTINATION_URL
              value: "http://podinfo-ap-southeast-{DST}.infrastructure-engineering.svc.cluster.local:9898"
            - name: REQUEST_PATH
              value: "/"
            - name: REQUEST_METHOD
              value: "GET"
            - name: REQUESTS_PER_SECOND
              value: "10"
            - name: METRICS_PORT
              value: "9090"
            - name: REQUEST_TIMEOUT
              value: "5s"
---
apiVersion: v1
kind: Service
metadata:
  name: zone-latency-client-{SRC}-to-{DST}
  namespace: infrastructure-engineering
  labels:
    app: zone-latency-client
    source-zone: ap-southeast-{SRC}
    destination-zone: ap-southeast-{DST}
spec:
  selector:
    app: zone-latency-client
    source-zone: ap-southeast-{SRC}
    destination-zone: ap-southeast-{DST}
  ports:
    - port: 9090
      targetPort: metrics
      name: metrics
```

Create files for all 9 combinations:
- `{SRC}=5a, {DST}=5a`
- `{SRC}=5a, {DST}=5b`
- `{SRC}=5a, {DST}=5c`
- `{SRC}=5b, {DST}=5a`
- `{SRC}=5b, {DST}=5b`
- `{SRC}=5b, {DST}=5c`
- `{SRC}=5c, {DST}=5a`
- `{SRC}=5c, {DST}=5b`
- `{SRC}=5c, {DST}=5c`

- [ ] **Step 2: Commit**

```bash
git add client/
git commit -m "feat: add 9 client deployment manifests for all zone combinations"
```

---

### Task 7: Client ServiceMonitor

**Files:**
- Create: `client/servicemonitor.yaml`

- [ ] **Step 1: Write client ServiceMonitor**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: zone-latency-client
  namespace: infrastructure-engineering
  labels:
    app: zone-latency-client
spec:
  selector:
    matchLabels:
      app: zone-latency-client
  endpoints:
    - port: metrics
      path: /metrics
      interval: 15s
```

- [ ] **Step 2: Commit**

```bash
git add client/servicemonitor.yaml
git commit -m "feat: add client ServiceMonitor"
```

---
