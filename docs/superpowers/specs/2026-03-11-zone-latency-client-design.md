# Zone Latency Client — Design Spec

## Overview

A Go client that continuously sends HTTP traffic between Kubernetes zones and exposes latency metrics for Prometheus scraping.

## Architecture

Single Go binary with two concerns:

- **Traffic sender**: goroutine with `time.Ticker` at `1s / RPS` interval, makes HTTP requests to the configured destination
- **Metrics server**: HTTP server on `METRICS_PORT` serving `/metrics` via `promhttp`

## Configuration

All configuration via environment variables.

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SOURCE_ZONE` | — | yes | Source zone label (e.g. `ap-southeast-5a`) |
| `DESTINATION_ZONE` | — | yes | Destination zone label |
| `DESTINATION_URL` | — | yes | Full URL of destination service |
| `REQUEST_PATH` | `/` | no | HTTP path to request |
| `REQUEST_METHOD` | `GET` | no | HTTP method |
| `REQUESTS_PER_SECOND` | `10` | no | Target requests per second |
| `METRICS_PORT` | `9090` | no | Port for Prometheus metrics endpoint |
| `REQUEST_TIMEOUT` | `5s` | no | HTTP request timeout |

## Metrics

| Metric | Type | Labels |
|--------|------|--------|
| `zone_latency_request_duration_seconds` | Histogram | source_zone, destination_zone, method, path, status_code |
| `zone_latency_requests_total` | Counter | source_zone, destination_zone, method, path, status_code |
| `zone_latency_request_errors_total` | Counter | source_zone, destination_zone, method, path |
| `zone_latency_requests_in_flight` | Gauge | source_zone, destination_zone |

## Project Structure

```
kubernetes-zone-latency-exporter/
├── main.go                          # entry point, config, ticker + metrics server
├── go.mod
├── go.sum
├── Dockerfile                       # multi-stage build, distroless/scratch runtime
├── server/
│   ├── deployment-zone-a.yaml       # podinfo in ap-southeast-5a
│   ├── deployment-zone-b.yaml       # podinfo in ap-southeast-5b
│   ├── deployment-zone-c.yaml       # podinfo in ap-southeast-5c
│   └── servicemonitor.yaml          # podinfo ServiceMonitor
├── client/
│   ├── deployment-5a-to-5a.yaml     # 9 client deployments (all zone combos)
│   ├── deployment-5a-to-5b.yaml
│   ├── deployment-5a-to-5c.yaml
│   ├── deployment-5b-to-5a.yaml
│   ├── deployment-5b-to-5b.yaml
│   ├── deployment-5b-to-5c.yaml
│   ├── deployment-5c-to-5a.yaml
│   ├── deployment-5c-to-5b.yaml
│   ├── deployment-5c-to-5c.yaml
│   └── servicemonitor.yaml          # client ServiceMonitor
```

## Deployment Strategy

- **3 podinfo servers** (one per zone) — existing manifests in `server/`
- **9 client deployments** (all zone combinations including self) in `client/`
- All resources in `infrastructure-engineering` namespace
- Each deployment uses `nodeAffinity` (`requiredDuringSchedulingIgnoredDuringExecution`) to pin to the correct source zone
- Client naming: `zone-latency-client-{src}-to-{dst}` (e.g. `zone-latency-client-5a-to-5b`)
- Destination URL pattern: `http://podinfo-ap-southeast-{dst}.infrastructure-engineering.svc.cluster.local:9898`

## Client ServiceMonitor

- Matches label `app: zone-latency-client`
- Scrapes `/metrics` on port `9090`
- Namespace: `infrastructure-engineering`

## Decisions

- **Ticker-based loop** over worker pool or load testing library — simplicity over precision, adequate for latency measurement
- **Single main.go** — logic is simple enough to not warrant packages
- **Environment variables only** — natural fit for Kubernetes deployments
- **5s default timeout** — generous for intra-region traffic, avoids hanging connections
