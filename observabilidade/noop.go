package observabilidade

import (
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
)

func noopMeter() metric.Meter {
	return metricnoop.NewMeterProvider().Meter("noop")
}
