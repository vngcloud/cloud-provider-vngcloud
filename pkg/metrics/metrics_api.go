package metrics

import (
	"sync"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

var (
	APIRequestMetrics = &VContainerMetrics{
		Duration: metrics.NewHistogramVec(
			&metrics.HistogramOpts{
				Name: "vcontainer_api_request_duration_seconds",
				Help: "Latency of an vContainer API call",
			}, []string{"request"}),
		Total: metrics.NewCounterVec(
			&metrics.CounterOpts{
				Name: "vcontainer_api_requests_total",
				Help: "Total number of vContainer API calls",
			}, []string{"request"}),
		Errors: metrics.NewCounterVec(
			&metrics.CounterOpts{
				Name: "vcontainer_api_request_errors_total",
				Help: "Total number of errors for an vContainer API call",
			}, []string{"request"}),
	}
)

var registerAPIMetrics sync.Once

// RegisterMetrics registers OpenStack metrics.
func doRegisterAPIMetrics() {
	registerAPIMetrics.Do(func() {
		legacyregistry.MustRegister(
			APIRequestMetrics.Duration,
			APIRequestMetrics.Total,
			APIRequestMetrics.Errors,
		)
	})
}
