package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
