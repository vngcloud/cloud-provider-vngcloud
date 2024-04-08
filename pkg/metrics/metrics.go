package metrics

import (
	"time"

	"k8s.io/component-base/metrics"
)

type VContainerMetrics struct {
	Duration *metrics.HistogramVec
	Total    *metrics.CounterVec
	Errors   *metrics.CounterVec
}

// MetricContext indicates the context for OpenStack metrics.
type MetricContext struct {
	Start      time.Time
	Attributes []string
	Metrics    *VContainerMetrics
}

// Observe records the request latency and counts the errors.
func (s *MetricContext) Observe(om *VContainerMetrics, err error) error {
	if om == nil {
		// mc.RequestMetrics not set, ignore this request
		return nil
	}

	om.Duration.WithLabelValues(s.Attributes...).Observe(
		time.Since(s.Start).Seconds())
	om.Total.WithLabelValues(s.Attributes...).Inc()
	if err != nil {
		om.Errors.WithLabelValues(s.Attributes...).Inc()
	}
	return err
}

// ObserveRequest records the request latency and counts the errors.
func (s *MetricContext) ObserveRequest(err error) error {
	return s.Observe(APIRequestMetrics, err)
}

// ObserveReconcile records the request reconciliation duration
func (s *MetricContext) ObserveReconcile(err error) error {
	return s.Observe(vccmReconcileMetrics, err)
}

// NewMetricContext creates a new MetricContext.
func NewMetricContext(resource string, request string) *MetricContext {
	return &MetricContext{
		Start:      time.Now(),
		Attributes: []string{resource + "_" + request},
	}
}

func RegisterMetrics(component string) {
	doRegisterAPIMetrics()
	if component == "occm" {
		doRegisterOccmMetrics()
	}
}
