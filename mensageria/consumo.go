package mensageria

import (
	"context"
	"fmt"
	"time"

	"github.com/deelperp/deelp-pkg/observabilidade"
	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const headerTentativasConsumo = "x-tentativas"

var (
	duracaoConsumo = observabilidade.HistogramaSegundos("deelp_mq_consumo_duracao_seconds", "Duração do processamento de mensagem por fila")
	latenciaFila   = observabilidade.HistogramaSegundos("deelp_mq_entrega_latencia_seconds", "Tempo entre publicar e começar a processar")
	enviadasDLQ    = observabilidade.Contador("deelp_mq_dlq_total", "Mensagens enviadas à DLQ por fila")
	reenfileiradas = observabilidade.Contador("deelp_mq_retry_total", "Mensagens reenfileiradas para nova tentativa por fila")
)

// CarrierAMQP adapta os headers da entrega para o propagador do OTel.
type CarrierAMQP amqp091.Table

func (c CarrierAMQP) Get(chave string) string {
	if s, ok := c[chave].(string); ok {
		return s
	}
	return ""
}

func (c CarrierAMQP) Set(chave, valor string) { c[chave] = valor }

func (c CarrierAMQP) Keys() []string {
	chaves := make([]string, 0, len(c))
	for k := range c {
		chaves = append(chaves, k)
	}
	return chaves
}

// ProcessarEntrega extrai o contexto do produtor, abre o span de consumo e mede o
// processamento. O desfecho (ack/retry/DLQ) continua com o chamador, que o informa
// em RegistrarRetry/RegistrarDLQ.
func ProcessarEntrega(ctx context.Context, fila string, d amqp091.Delivery, handler func(context.Context) error) error {
	return ProcessarMensagem(ctx, fila, d.Headers, d.MessageId, d.Timestamp, handler)
}

// ProcessarMensagem é ProcessarEntrega para quem já perdeu a amqp091.Delivery e só
// guardou os headers e metadados da entrega.
func ProcessarMensagem(ctx context.Context, fila string, headers amqp091.Table, messageId string, ts time.Time, handler func(context.Context) error) error {
	ctx = otel.GetTextMapPropagator().Extract(ctx, CarrierAMQP(headers))
	ctx, span := otel.Tracer("github.com/deelperp/deelp-pkg/mensageria").Start(ctx, "consumir "+fila,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", fila),
			attribute.String("messaging.message.id", observabilidade.RedigirTexto(messageId)),
			attribute.Int("messaging.deelp.tentativa", tentativaDe(headers)+1),
		))
	attrs := metric.WithAttributes(attribute.String("fila", fila))
	if !ts.IsZero() {
		latenciaFila.Record(ctx, time.Since(ts).Seconds(), attrs)
	}
	inicio := time.Now()
	registrar := func(err error) {
		duracaoConsumo.Record(ctx, time.Since(inicio).Seconds(), metric.WithAttributes(
			attribute.String("fila", fila), attribute.String("resultado", observabilidade.ResultadoDe(err))))
	}
	defer func() {
		if p := recover(); p != nil {
			erro := fmt.Errorf("panic: %v", p)
			registrar(erro)
			observabilidade.FinalizarSpanErr(span, erro)
			panic(p)
		}
	}()
	err := handler(ctx)
	registrar(err)
	observabilidade.FinalizarSpanErr(span, err)
	return err
}

func RegistrarRetry(ctx context.Context, fila string) {
	reenfileiradas.Add(ctx, 1, metric.WithAttributes(attribute.String("fila", fila)))
}

func RegistrarDLQ(ctx context.Context, fila string) {
	enviadasDLQ.Add(ctx, 1, metric.WithAttributes(attribute.String("fila", fila)))
}

func tentativaDe(h amqp091.Table) int {
	switch v := h[headerTentativasConsumo].(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// CabecalhosDoContexto devolve os headers AMQP com o traceparent do contexto, para o
// consumidor continuar o trace do produtor.
func CabecalhosDoContexto(ctx context.Context) amqp091.Table {
	headers := amqp091.Table{}
	otel.GetTextMapPropagator().Inject(ctx, CarrierAMQP(headers))
	return headers
}
