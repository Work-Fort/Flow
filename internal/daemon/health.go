// SPDX-License-Identifier: GPL-2.0-only
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

// HealthStatus represents the overall system health.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// CheckSeverity classifies a check result.
type CheckSeverity string

const (
	SeverityOK      CheckSeverity = "ok"
	SeverityWarning CheckSeverity = "warning"
	SeverityError   CheckSeverity = "error"
)

// CheckResult holds the outcome of a single health check.
type CheckResult struct {
	Name     string        `json:"name"`
	Severity CheckSeverity `json:"severity"`
	Message  string        `json:"message,omitempty"`
}

// HealthReport is returned by the health endpoint.
type HealthReport struct {
	Status string        `json:"status"`
	Checks []CheckResult `json:"checks"`
}

// CheckFunc is a health check function. It returns a CheckResult.
type CheckFunc func(ctx context.Context) CheckResult

// registeredCheck pairs a name and function for a single health check.
type registeredCheck struct {
	name string
	fn   CheckFunc
}

// HealthService is a registry for named health checks.
type HealthService struct {
	mu       sync.RWMutex
	results  map[string]CheckResult
	periodic []registeredCheck
}

// NewHealthService creates a new HealthService.
func NewHealthService() *HealthService {
	return &HealthService{
		results: make(map[string]CheckResult),
	}
}

// RegisterBootCheck runs fn immediately and records the result under name.
func (h *HealthService) RegisterBootCheck(name string, fn CheckFunc) {
	result := fn(context.Background())
	result.Name = name
	h.mu.Lock()
	h.results[name] = result
	h.mu.Unlock()
}

// RegisterPeriodicCheck records a check that will be run by StartPeriodic.
// It also runs the check immediately so the result is available before the first tick.
func (h *HealthService) RegisterPeriodicCheck(name string, fn CheckFunc) {
	result := fn(context.Background())
	result.Name = name
	h.mu.Lock()
	h.results[name] = result
	h.periodic = append(h.periodic, registeredCheck{name: name, fn: fn})
	h.mu.Unlock()
}

// StartPeriodic runs all periodic checks on the given interval until ctx is cancelled.
func (h *HealthService) StartPeriodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.mu.RLock()
			checks := make([]registeredCheck, len(h.periodic))
			copy(checks, h.periodic)
			h.mu.RUnlock()

			for _, c := range checks {
				result := c.fn(ctx)
				result.Name = c.name
				h.mu.Lock()
				h.results[c.name] = result
				h.mu.Unlock()
			}
		}
	}
}

// AddWarning records a warning check result.
func (h *HealthService) AddWarning(msg string) {
	name := fmt.Sprintf("warning:%s", msg)
	result := CheckResult{Name: name, Severity: SeverityWarning, Message: msg}
	h.mu.Lock()
	h.results[name] = result
	h.mu.Unlock()
}

// AddError records an error check result.
func (h *HealthService) AddError(msg string) {
	name := fmt.Sprintf("error:%s", msg)
	result := CheckResult{Name: name, Severity: SeverityError, Message: msg}
	h.mu.Lock()
	h.results[name] = result
	h.mu.Unlock()
}

// Report computes and returns the current health report.
func (h *HealthService) Report() HealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()

	checks := make([]CheckResult, 0, len(h.results))
	overall := StatusHealthy

	for _, r := range h.results {
		checks = append(checks, r)
		switch r.Severity {
		case SeverityError:
			overall = StatusUnhealthy
		case SeverityWarning:
			if overall == StatusHealthy {
				overall = StatusDegraded
			}
		}
	}

	sort.Slice(checks, func(i, j int) bool {
		return checks[i].Name < checks[j].Name
	})

	return HealthReport{
		Status: string(overall),
		Checks: checks,
	}
}

// HandleHealth returns an http.HandlerFunc for the GET /v1/health endpoint.
func HandleHealth(health *HealthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := health.Report()

		var statusCode int
		switch report.Status {
		case string(StatusHealthy):
			statusCode = http.StatusOK
		case string(StatusDegraded):
			statusCode = 218
		default:
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(report) //nolint:errcheck
	}
}

// HandleUIHealth returns an http.HandlerFunc for Pylon service discovery.
func HandleUIHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"name":  "flow",
			"label": "Flow",
			"route": "/flow",
		}) //nolint:errcheck
	}
}
