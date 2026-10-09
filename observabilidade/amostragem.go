package observabilidade

import (
	"os"
	"strconv"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const envAmostragem = "DEELP_TRACE_SAMPLE_RATIO"

func amostrador(fracao float64) sdktrace.Sampler {
	if fracao <= 0 {
		fracao, _ = strconv.ParseFloat(os.Getenv(envAmostragem), 64)
	}
	if fracao <= 0 || fracao >= 1 {
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(fracao))
}
