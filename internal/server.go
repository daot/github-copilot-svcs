package internal

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// Constants for timeout values
const (
	shutdownTimeout = 10 * time.Second

	// Optimized HTTP client configuration for better performance
	maxIdleConns        = 200 // Increased for better connection reuse
	maxIdleConnsPerHost = 50  // Increased for high-traffic scenarios
	maxConnsPerHost     = 100 // Limit concurrent connections per host
	workerMultiplier    = 2
)

// Metrics holds server performance metrics
type Metrics struct {
	RequestsTotal     int64
	RequestsDuration  float64
	ActiveConnections int64
	mutex             sync.RWMutex
}

// Server represents the HTTP server and its dependencies
type Server struct {
	config     *Config
	httpServer *http.Server
	httpClient *http.Client
	workerPool *WorkerPool
	metrics    *Metrics
}

// WorkerPool handles background processing
type WorkerPool struct {
	workers  int
	jobQueue chan func()
	quit     chan bool
	wg       sync.WaitGroup
}

// NewWorkerPool creates a new worker pool with intelligent sizing
func NewWorkerPool(workers int) *WorkerPool {
	if workers <= 0 {
		// Intelligent sizing based on system resources and workload
		cpuCount := runtime.NumCPU()
		// Use 50% of CPU cores for workers, with a minimum of 2 and maximum of 16
		workers = cpuCount / 2
		if workers < 2 {
			workers = 2
		}
		if workers > 16 {
			workers = 16
		}
	}

	// Increased buffer size for better burst handling
	bufferSize := workers * workerMultiplier * 2

	wp := &WorkerPool{
		workers:  workers,
		jobQueue: make(chan func(), bufferSize), // Buffer for burst traffic
		quit:     make(chan bool),
	}

	wp.start()
	return wp
}

func (wp *WorkerPool) start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go func() {
			defer wp.wg.Done()
			for {
				select {
				case job := <-wp.jobQueue:
					job()
				case <-wp.quit:
					return
				}
			}
		}()
	}
}

// Submit adds a job to the worker pool
func (wp *WorkerPool) Submit(job func()) {
	wp.jobQueue <- job
}

// Stop gracefully stops the worker pool
func (wp *WorkerPool) Stop() {
	close(wp.quit)
	wp.wg.Wait()
}

// CreateHTTPClient creates a configured HTTP client with optimized connection pooling
func CreateHTTPClient(cfg *Config) *http.Client {
	return &http.Client{
		Timeout: time.Duration(cfg.Timeouts.HTTPClient) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        maxIdleConns,
			MaxIdleConnsPerHost: maxIdleConnsPerHost,
			MaxConnsPerHost:     maxConnsPerHost,
			IdleConnTimeout:     time.Duration(cfg.Timeouts.IdleConnTimeout) * time.Second,
			DisableKeepAlives:   false, // Enable keep-alives for better performance
			DisableCompression:  false, // Enable compression for better performance
			DialContext: (&net.Dialer{
				Timeout:   time.Duration(cfg.Timeouts.DialTimeout) * time.Second,
				KeepAlive: time.Duration(cfg.Timeouts.KeepAlive) * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: time.Duration(cfg.Timeouts.TLSHandshake) * time.Second,
		},
	}
}

// NewServer creates a new server instance
func NewServer(cfg *Config, httpClient *http.Client) *Server {
	workerPool := NewWorkerPool(runtime.NumCPU() * workerMultiplier)

	// Initialize metrics
	metrics := &Metrics{}

	// Create auth service
	authService := NewAuthService(httpClient)

	// Create coalescing cache for models
	coalescingCache := NewCoalescingCache()
	modelsService := NewModelsService(coalescingCache, httpClient)

	// Create proxy service
	proxyService := NewProxyService(cfg, httpClient, authService, workerPool)

	// Create health checker
	healthChecker := NewHealthChecker(httpClient, "dev") // TODO: get version from build

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", modelsService.Handler())
	mux.HandleFunc("/v1/chat/completions", proxyService.Handler())
	mux.HandleFunc("/health", healthChecker.Handler())
	mux.HandleFunc("/metrics", metrics.Handler()) // Add metrics endpoint

	// Add pprof endpoints for profiling
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/cmdline", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/profile", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/symbol", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/trace", http.DefaultServeMux.ServeHTTP)

	port := cfg.Port
	if port == 0 {
		port = 8081 // default port
	}

	// Build middleware chain
	var handler http.Handler = mux

	// Apply middleware in reverse order (last applied = first executed)
	handler = SecurityHeadersMiddleware(handler)
	handler = CORSMiddleware(cfg)(handler)
	handler = LoggingMiddleware(handler)
	handler = RecoveryMiddleware(handler)
	handler = CompressionMiddleware()(handler)   // Add compression for better performance
	handler = metrics.MetricsMiddleware(handler) // Add metrics collection
	// Note: TimeoutMiddleware could be added here if needed per-request timeouts
	// handler = TimeoutMiddleware(time.Duration(cfg.Timeouts.ProxyContext) * time.Second)(handler)

	// Configure HTTP/2 support with optimized TLS settings
	tlsConfig := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP256, tls.CurveP384, tls.CurveP521},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		},
	}

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.Timeouts.ServerRead) * time.Second,
		WriteTimeout: time.Duration(cfg.Timeouts.ServerWrite) * time.Second,
		IdleTimeout:  time.Duration(cfg.Timeouts.ServerIdle) * time.Second,
		TLSConfig:    tlsConfig,
		// Enable HTTP/2 support (empty map disables HTTP/1.1 fallback to HTTP/2)
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	return &Server{
		config:     cfg,
		httpServer: httpServer,
		httpClient: httpClient,
		workerPool: workerPool,
		metrics:    metrics,
	}
}

// Start starts the HTTP server with graceful shutdown
func (s *Server) Start() error {
	s.setupGracefulShutdown()

	port := s.config.Port
	if port == 0 {
		port = 8081
	}

	fmt.Printf("Starting GitHub Copilot proxy server on port %d...\n", port)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  - Models: http://localhost:%d/v1/models\n", port)
	fmt.Printf("  - Chat: http://localhost:%d/v1/chat/completions\n", port)
	fmt.Printf("  - Health: http://localhost:%d/health\n", port)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %v", err)
	}

	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	fmt.Println("Stopping worker pool...")
	s.workerPool.Stop()
	fmt.Println("Worker pool stopped.")

	fmt.Println("Shutting down HTTP server...")
	err := s.httpServer.Shutdown(ctx)
	if err != nil {
		fmt.Printf("Error during HTTP server shutdown: %v\n", err)
		return err
	}
	fmt.Println("HTTP server shutdown complete.")

	return nil
}

func (s *Server) setupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\nGracefully shutting down...")

		if err := s.Stop(); err != nil {
			Error("Server shutdown error", "error", err)
		}
	}()
}

// healthHandler is now replaced by the comprehensive HealthChecker

// MetricsMiddleware adds request metrics collection
func (m *Metrics) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Track active connections
		m.mutex.Lock()
		m.ActiveConnections++
		m.mutex.Unlock()

		// Wrap response writer to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: 200}

		// Process request
		next.ServeHTTP(rw, r)

		// Record metrics
		duration := time.Since(start).Seconds()
		m.mutex.Lock()
		m.RequestsTotal++
		m.RequestsDuration += duration
		m.ActiveConnections--
		m.mutex.Unlock()
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Handler returns metrics in Prometheus format
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mutex.RLock()
		requestsTotal := m.RequestsTotal
		requestsDuration := m.RequestsDuration
		activeConnections := m.ActiveConnections
		m.mutex.RUnlock()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		fmt.Fprintf(w, "# HELP github_copilot_requests_total Total number of requests\n")
		fmt.Fprintf(w, "# TYPE github_copilot_requests_total counter\n")
		fmt.Fprintf(w, "github_copilot_requests_total %d\n", requestsTotal)

		fmt.Fprintf(w, "# HELP github_copilot_requests_duration_seconds Total duration of requests in seconds\n")
		fmt.Fprintf(w, "# TYPE github_copilot_requests_duration_seconds counter\n")
		fmt.Fprintf(w, "github_copilot_requests_duration_seconds %f\n", requestsDuration)

		fmt.Fprintf(w, "# HELP github_copilot_active_connections Current number of active connections\n")
		fmt.Fprintf(w, "# TYPE github_copilot_active_connections gauge\n")
		fmt.Fprintf(w, "github_copilot_active_connections %d\n", activeConnections)

		// Add uptime metric
		uptime := time.Since(startTime).Seconds()
		fmt.Fprintf(w, "# HELP github_copilot_uptime_seconds Server uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE github_copilot_uptime_seconds counter\n")
		fmt.Fprintf(w, "github_copilot_uptime_seconds %f\n", uptime)
	}
}

var startTime = time.Now()
