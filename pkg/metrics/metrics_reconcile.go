package metrics

import (
	"sync"

	"k8s.io/component-base/metrics"
	"k8s.io/component-base/metrics/legacyregistry"
)

var (
	vccmReconcileMetrics = &VContainerMetrics{
		Duration: metrics.NewHistogramVec(
			&metrics.HistogramOpts{
				Name:    "cloudprovider_vcontainer_reconcile_duration_seconds",
				Help:    "Time taken by various parts of vContainer cloud controller manager reconciliation loops",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.5, 5.0, 7.5, 10.0, 12.5, 15.0, 17.5, 20.0, 22.5, 25.0, 27.5, 30.0, 50.0, 75.0, 100.0, 1000.0},
			}, []string{"operation"}),
		Total: metrics.NewCounterVec(
			&metrics.CounterOpts{
				Name: "cloudprovider_vcontainer_reconcile_total",
				Help: "Total number of vContainer cloud controller manager reconciliations",
			}, []string{"operation"}),
		Errors: metrics.NewCounterVec(
			&metrics.CounterOpts{
				Name: "cloudprovider_vcontainer_reconcile_errors_total",
				Help: "Total number of vContainer cloud controller manager reconciliation errors",
			}, []string{"operation"}),
	}
)

var registerOccmMetrics sync.Once

// RegisterMetrics registers OpenStack metrics.
func doRegisterOccmMetrics() {
	registerOccmMetrics.Do(func() {
		legacyregistry.MustRegister(
			vccmReconcileMetrics.Duration,
			vccmReconcileMetrics.Total,
			vccmReconcileMetrics.Errors,
		)
	})
}
