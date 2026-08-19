package platform

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func MetricsHandler(service, version string) (http.Handler, error) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_vision_build_info",
		Help: "Build information for a new-vision service.",
		ConstLabels: prometheus.Labels{
			"service": service,
			"version": version,
		},
	})
	buildInfo.Set(1)
	if err := registry.Register(buildInfo); err != nil {
		return nil, fmt.Errorf("register build metric: %w", err)
	}
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), nil
}
