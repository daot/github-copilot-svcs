package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

// Constants for health checking
const (
	healthCheckTimeout = 10 * time.Second
	memoryThresholdMB  = 1024                   // 1GB in MB
	memoryWarningGB    = 1024 * 1024 * 1024     // 1GB
	memoryCriticalGB   = 2 * 1024 * 1024 * 1024 // 2GB
	goroutineWarning   = 1000
	goroutineCritical  = 5000
	bytesToMB          = 1024 * 1024
	percentMultiplier  = 100
)

// HealthStatus represents the overall health status
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck represents a single health check
type HealthCheck struct {
	Name        string                 `json:"name"`
	Status      HealthStatus           `json:"status"`
	Message     string                 `json:"message,omitempty"`
	Duration    time.Duration          `json:"duration"`
	LastChecked time.Time              `json:"last_checked"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// HealthResponse represents the complete health response
type HealthResponse struct {
	Status    HealthStatus           `json:"status"`
	Service   string                 `json:"service"`
	Version   string                 `json:"version,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Uptime    time.Duration          `json:"uptime"`
	Checks    []HealthCheck          `json:"checks"`
	System    SystemMetrics          `json:"system"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// SystemMetrics represents system-level metrics
type SystemMetrics struct {
	Memory     MemoryMetrics `json:"memory"`
	Goroutines int           `json:"goroutines"`
	CGoCalls   int64         `json:"cgo_calls"`
	NumCPU     int           `json:"num_cpu"`
	GOMAXPROCS int           `json:"gomaxprocs"`
}

// MemoryMetrics represents memory usage metrics
type MemoryMetrics struct {
	Alloc        uint64  `json:"alloc"`          // bytes allocated and still in use
	TotalAlloc   uint64  `json:"total_alloc"`    // bytes allocated (even if freed)
	Sys          uint64  `json:"sys"`            // bytes obtained from system
	Lookups      uint64  `json:"lookups"`        // number of pointer lookups
	Mallocs      uint64  `json:"mallocs"`        // number of mallocs
	Frees        uint64  `json:"frees"`          // number of frees
	HeapAlloc    uint64  `json:"heap_alloc"`     // bytes allocated and still in use
	HeapSys      uint64  `json:"heap_sys"`       // bytes obtained from system
	HeapIdle     uint64  `json:"heap_idle"`      // bytes in idle spans
	HeapInuse    uint64  `json:"heap_inuse"`     // bytes in non-idle span
	HeapReleased uint64  `json:"heap_released"`  // bytes released to the OS
	GCCPUPercent float64 `json:"gc_cpu_percent"` // percentage of CPU time spent in GC
}

// HealthChecker manages health checks
type HealthChecker struct {
	startTime  time.Time
	httpClient *http.Client
	version    string
	checks     []HealthCheckFunc
}

// HealthCheckFunc represents a health check function
type HealthCheckFunc func(ctx context.Context) HealthCheck

// NewHealthChecker creates a new health checker
func NewHealthChecker(httpClient *http.Client, version string) *HealthChecker {
	hc := &HealthChecker{
		startTime:  time.Now(),
		httpClient: httpClient,
		version:    version,
		checks:     make([]HealthCheckFunc, 0),
	}

	// Add default health checks
	hc.AddCheck(hc.checkMemory)
	hc.AddCheck(hc.checkGoroutines)

	return hc
}

// AddCheck adds a health check function
func (hc *HealthChecker) AddCheck(check HealthCheckFunc) {
	hc.checks = append(hc.checks, check)
}

// CheckHealth performs all health checks and returns the overall status
func (hc *HealthChecker) CheckHealth(ctx context.Context) *HealthResponse {
	start := time.Now()

	// Run all checks
	checks := make([]HealthCheck, 0, len(hc.checks))
	overallStatus := StatusHealthy

	for _, checkFunc := range hc.checks {
		check := checkFunc(ctx)
		checks = append(checks, check)

		// Determine overall status
		if check.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if check.Status == StatusDegraded && overallStatus == StatusHealthy {
			overallStatus = StatusDegraded
		}
	}

	// Collect system metrics
	systemMetrics := hc.collectSystemMetrics()

	response := &HealthResponse{
		Status:    overallStatus,
		Service:   "github-copilot-svcs",
		Version:   hc.version,
		Timestamp: time.Now(),
		Uptime:    time.Since(hc.startTime),
		Checks:    checks,
		System:    systemMetrics,
		Details: map[string]interface{}{
			"health_check_duration": time.Since(start),
		},
	}

	return response
}

// HTTP handler for health checks
func (hc *HealthChecker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		health := hc.CheckHealth(ctx)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		// Set HTTP status based on health status
		switch health.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusDegraded:
			w.WriteHeader(http.StatusOK) // Still OK but degraded
		case StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		if err := json.NewEncoder(w).Encode(health); err != nil {
			Error("Failed to encode health response", "error", err)
			WriteInternalError(w)
		}
	}
}

// Default health checks
func (hc *HealthChecker) checkMemory(_ context.Context) HealthCheck {
	start := time.Now()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	status := StatusHealthy
	message := "Memory usage normal"

	// Check if memory usage is concerning
	if m.Alloc > memoryWarningGB {
		status = StatusDegraded
		message = "High memory usage detected"
	}

	// Check if memory usage is critical
	if m.Alloc > memoryCriticalGB {
		status = StatusUnhealthy
		message = "Critical memory usage detected"
	}

	return HealthCheck{
		Name:        "memory",
		Status:      status,
		Message:     message,
		Duration:    time.Since(start),
		LastChecked: time.Now(),
		Details: map[string]interface{}{
			"alloc_mb":      m.Alloc / bytesToMB,
			"sys_mb":        m.Sys / bytesToMB,
			"heap_alloc_mb": m.HeapAlloc / bytesToMB,
			"num_gc":        m.NumGC,
		},
	}
}

func (hc *HealthChecker) checkGoroutines(_ context.Context) HealthCheck {
	start := time.Now()

	numGoroutines := runtime.NumGoroutine()

	status := StatusHealthy
	message := "Goroutine count normal"

	// Check if goroutine count is concerning
	if numGoroutines > goroutineWarning {
		status = StatusDegraded
		message = "High goroutine count detected"
	}

	// Check if goroutine count is critical
	if numGoroutines > goroutineCritical {
		status = StatusUnhealthy
		message = "Critical goroutine count detected"
	}

	return HealthCheck{
		Name:        "goroutines",
		Status:      status,
		Message:     message,
		Duration:    time.Since(start),
		LastChecked: time.Now(),
		Details: map[string]interface{}{
			"count": numGoroutines,
		},
	}
}

func (hc *HealthChecker) collectSystemMetrics() SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SystemMetrics{
		Memory: MemoryMetrics{
			Alloc:        m.Alloc,
			TotalAlloc:   m.TotalAlloc,
			Sys:          m.Sys,
			Lookups:      m.Lookups,
			Mallocs:      m.Mallocs,
			Frees:        m.Frees,
			HeapAlloc:    m.HeapAlloc,
			HeapSys:      m.HeapSys,
			HeapIdle:     m.HeapIdle,
			HeapInuse:    m.HeapInuse,
			HeapReleased: m.HeapReleased,
			GCCPUPercent: m.GCCPUFraction * percentMultiplier,
		},
		Goroutines: runtime.NumGoroutine(),
		CGoCalls:   runtime.NumCgoCall(),
		NumCPU:     runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
	}
}
