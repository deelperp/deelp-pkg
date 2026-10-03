package mensageria

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrNaoRoteada         = errors.New("mensageria: mensagem sem fila de destino")
	ErrRecusada           = errors.New("mensageria: broker recusou a mensagem")
	ErrConfirmacaoIncerta = errors.New("mensageria: sem confirmação do broker; a mensagem pode ou não ter sido aceita")
	ErrPublisherFechado   = errors.New("mensageria: publisher fechado")
)

const confirmacaoPadrao = 5 * time.Second

type ProvedorCanal interface {
	Channel() (*amqp091.Channel, error)
}

type PublisherConfig struct {
	TempoConfirmacao time.Duration
}

type Mensagem struct {
	Exchange        string
	ChaveRoteamento string
	Corpo           []byte
	ContentType     string
	Headers         amqp091.Table
	MessageId       string
	Transiente      bool
	PermitirSemRota bool
}

type Publisher struct {
	provedor ProvedorCanal
	tempo    time.Duration

	mu        sync.Mutex
	canal     *amqp091.Channel
	retornos  chan amqp091.Return
	fechado   bool
	declarado map[string]struct{}
}

func NovoPublisher(provedor ProvedorCanal, cfg PublisherConfig) *Publisher {
	tempo := cfg.TempoConfirmacao
	if tempo <= 0 {
		tempo = confirmacaoPadrao
	}
	return &Publisher{provedor: provedor, tempo: tempo, declarado: map[string]struct{}{}}
}

// DeclararExchange declara a exchange topic durável uma vez por canal.
func (p *Publisher) DeclararExchange(nome string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	canal, err := p.canalLocked()
	if err != nil {
		return err
	}
	return p.declararLocked(canal, nome)
}

func (p *Publisher) declararLocked(canal *amqp091.Channel, nome string) error {
	if nome == "" {
		return nil
	}
	if _, ok := p.declarado[nome]; ok {
		return nil
	}
	if err := canal.ExchangeDeclare(nome, "topic", true, false, false, false, nil); err != nil {
		p.descartarLocked()
		return fmt.Errorf("mensageria: declarar exchange %q: %w", nome, err)
	}
	p.declarado[nome] = struct{}{}
	return nil
}

// Publicar só retorna nil depois da confirmação do broker. Mensagem sem rota
// devolve ErrNaoRoteada, a menos que PermitirSemRota esteja marcado.
func (p *Publisher) Publicar(ctx context.Context, m Mensagem) (err error) {
	ctx, span := otel.Tracer("github.com/deelperp/deelp-pkg/mensageria").Start(ctx, "publicar "+m.Exchange,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", m.Exchange),
			attribute.String("messaging.rabbitmq.destination.routing_key", m.ChaveRoteamento),
		))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	if m.MessageId == "" {
		m.MessageId = uuid.NewString()
	}
	headers := amqp091.Table{}
	for k, v := range m.Headers {
		headers[k] = v
	}
	for k, v := range propagacao(ctx) {
		headers[k] = v
	}
	modo := amqp091.Persistent
	if m.Transiente {
		modo = amqp091.Transient
	}
	contentType := m.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	publicacao := amqp091.Publishing{
		Headers:      headers,
		ContentType:  contentType,
		DeliveryMode: modo,
		MessageId:    m.MessageId,
		Timestamp:    time.Now(),
		Body:         m.Corpo,
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	confirmacao, err := p.enviarLocked(ctx, m, publicacao)
	if err != nil {
		return err
	}

	esperaCtx, cancel := context.WithTimeout(ctx, p.tempo)
	defer cancel()
	aceita, err := confirmacao.WaitContext(esperaCtx)
	if err != nil {
		p.descartarLocked()
		return fmt.Errorf("%w: %v", ErrConfirmacaoIncerta, err)
	}
	if !aceita {
		return ErrRecusada
	}
	if p.retornadaLocked(m.MessageId) && !m.PermitirSemRota {
		return fmt.Errorf("%w: exchange %q, chave %q", ErrNaoRoteada, m.Exchange, m.ChaveRoteamento)
	}
	return nil
}

func (p *Publisher) enviarLocked(ctx context.Context, m Mensagem, publicacao amqp091.Publishing) (*amqp091.DeferredConfirmation, error) {
	var ultimo error
	for tentativa := 0; tentativa < 2; tentativa++ {
		canal, err := p.canalLocked()
		if err != nil {
			return nil, err
		}
		if err := p.declararLocked(canal, m.Exchange); err != nil {
			ultimo = err
			continue
		}
		confirmacao, err := canal.PublishWithDeferredConfirmWithContext(ctx, m.Exchange, m.ChaveRoteamento, true, false, publicacao)
		if err == nil {
			return confirmacao, nil
		}
		ultimo = err
		p.descartarLocked()
		if !errors.Is(err, amqp091.ErrClosed) {
			break
		}
	}
	return nil, fmt.Errorf("mensageria: publicar em %q: %w", m.Exchange, ultimo)
}

func (p *Publisher) retornadaLocked(messageId string) bool {
	for {
		select {
		case retorno, ok := <-p.retornos:
			if !ok {
				return false
			}
			if retorno.MessageId == messageId {
				return true
			}
		default:
			return false
		}
	}
}

func (p *Publisher) canalLocked() (*amqp091.Channel, error) {
	if p.fechado {
		return nil, ErrPublisherFechado
	}
	if p.canal != nil && !p.canal.IsClosed() {
		return p.canal, nil
	}
	p.descartarLocked()
	canal, err := p.provedor.Channel()
	if err != nil {
		return nil, fmt.Errorf("mensageria: abrir canal: %w", err)
	}
	if err := canal.Confirm(false); err != nil {
		_ = canal.Close()
		return nil, fmt.Errorf("mensageria: ativar confirmações: %w", err)
	}
	p.retornos = canal.NotifyReturn(make(chan amqp091.Return, 16))
	p.canal = canal
	return canal, nil
}

func (p *Publisher) descartarLocked() {
	if p.canal != nil {
		_ = p.canal.Close()
	}
	p.canal = nil
	p.retornos = nil
	p.declarado = map[string]struct{}{}
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fechado {
		return nil
	}
	p.fechado = true
	var err error
	if p.canal != nil && !p.canal.IsClosed() {
		err = p.canal.Close()
	}
	p.canal = nil
	p.retornos = nil
	return err
}

func propagacao(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier
}
