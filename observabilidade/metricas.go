package observabilidade

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const escopoMetricas = "github.com/deelperp/deelp-pkg/observabilidade"

var bucketsSegundos = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}

func visoesPadrao() []sdkmetric.View {
	return []sdkmetric.View{
		sdkmetric.NewView(
			sdkmetric.Instrument{Kind: sdkmetric.InstrumentKindHistogram, Unit: "s"},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: bucketsSegundos}},
		),
	}
}

func Meter() metric.Meter {
	return otel.Meter(escopoMetricas)
}

// Contador devolve um contador que nunca é nil: falha ao criar o instrumento vira noop.
func Contador(nome, descricao string) metric.Int64Counter {
	c, err := Meter().Int64Counter(nome, metric.WithDescription(descricao))
	if err != nil {
		c, _ = noopMeter().Int64Counter(nome)
	}
	return c
}

// Histograma em segundos usa os buckets de bucketsSegundos (view registrada em Iniciar).
func HistogramaSegundos(nome, descricao string) metric.Float64Histogram {
	h, err := Meter().Float64Histogram(nome, metric.WithDescription(descricao), metric.WithUnit("s"))
	if err != nil {
		h, _ = noopMeter().Float64Histogram(nome)
	}
	return h
}

func Medidor(nome, descricao string) metric.Float64Gauge {
	g, err := Meter().Float64Gauge(nome, metric.WithDescription(descricao))
	if err != nil {
		g, _ = noopMeter().Float64Gauge(nome)
	}
	return g
}

func EmAndamento(nome, descricao string) metric.Int64UpDownCounter {
	c, err := Meter().Int64UpDownCounter(nome, metric.WithDescription(descricao))
	if err != nil {
		c, _ = noopMeter().Int64UpDownCounter(nome)
	}
	return c
}
