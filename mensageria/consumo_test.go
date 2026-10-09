package mensageria

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestProcessarEntregaLigaSpanAoProdutor(t *testing.T) {
	gravador := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(gravador))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	ctxProdutor, produtor := tp.Tracer("t").Start(context.Background(), "publicar")
	headers := amqp091.Table{}
	otel.GetTextMapPropagator().Inject(ctxProdutor, CarrierAMQP(headers))
	produtor.End()

	falha := errors.New("falhou")
	err := ProcessarEntrega(context.Background(), "fila.teste", amqp091.Delivery{Headers: headers}, func(context.Context) error { return falha })
	if !errors.Is(err, falha) {
		t.Fatalf("erro do handler deve voltar intacto, veio %v", err)
	}

	var consumo sdktrace.ReadOnlySpan
	for _, s := range gravador.Ended() {
		if s.Name() == "consumir fila.teste" {
			consumo = s
		}
	}
	if consumo == nil {
		t.Fatal("span de consumo não foi criado")
	}
	if consumo.Parent().TraceID() != produtor.SpanContext().TraceID() {
		t.Fatal("span de consumo não pertence ao trace do produtor")
	}
	if consumo.Status().Code.String() != "Error" {
		t.Fatalf("status esperado Error, veio %s", consumo.Status().Code)
	}
}

func TestTentativaDe(t *testing.T) {
	casos := map[string]amqp091.Table{
		"int32": {headerTentativasConsumo: int32(2)},
		"int64": {headerTentativasConsumo: int64(2)},
		"vazio": {},
	}
	esperado := map[string]int{"int32": 2, "int64": 2, "vazio": 0}
	for nome, h := range casos {
		if got := tentativaDe(h); got != esperado[nome] {
			t.Errorf("%s: esperado %d, veio %d", nome, esperado[nome], got)
		}
	}
}
