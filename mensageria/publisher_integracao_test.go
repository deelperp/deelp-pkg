package mensageria

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func conexaoReal(t *testing.T) *amqp091.Connection {
	t.Helper()
	url := os.Getenv("DEELP_RABBITMQ_URL")
	if url == "" {
		t.Skip("DEELP_RABBITMQ_URL não definida; teste com broker real roda no CI")
	}
	conn, err := amqp091.Dial(url)
	if err != nil {
		t.Fatalf("broker indisponível: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func filaLigada(t *testing.T, conn *amqp091.Connection, exchange, chave string) <-chan amqp091.Delivery {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	q, err := ch.QueueDeclare("deelp.teste.fila."+uuid.NewString(), true, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.QueueBind(q.Name, chave, exchange, false, nil); err != nil {
		t.Fatal(err)
	}
	entregas, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return entregas
}

func exchangeDeTeste(t *testing.T) string {
	t.Helper()
	return "deelp.teste." + uuid.NewString()
}

func TestIntegracaoPublisher_ConfirmadaPersistenteComTrace(t *testing.T) {
	conn := conexaoReal(t)
	exchange := exchangeDeTeste(t)
	entregas := filaLigada(t, conn, exchange, "tarefa.criada")
	antigo := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(antigo) })
	ctx, span := sdktrace.NewTracerProvider().Tracer("teste").Start(context.Background(), "origem")
	defer span.End()

	p := NovoPublisher(conn, PublisherConfig{})
	defer p.Close()
	if err := p.Publicar(ctx, Mensagem{Exchange: exchange, ChaveRoteamento: "tarefa.criada", Corpo: []byte(`{"ok":true}`)}); err != nil {
		t.Fatal(err)
	}

	select {
	case d := <-entregas:
		if d.DeliveryMode != amqp091.Persistent || d.MessageId == "" || string(d.Body) != `{"ok":true}` {
			t.Fatalf("entrega inesperada: modo=%d id=%q corpo=%s", d.DeliveryMode, d.MessageId, d.Body)
		}
		if _, ok := d.Headers["traceparent"]; !ok {
			t.Fatal("trace não propagado")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mensagem confirmada não chegou à fila")
	}
}

func TestIntegracaoPublisher_SemRotaEhErro(t *testing.T) {
	conn := conexaoReal(t)
	p := NovoPublisher(conn, PublisherConfig{})
	defer p.Close()

	err := p.Publicar(context.Background(), Mensagem{Exchange: exchangeDeTeste(t), ChaveRoteamento: "ninguem.escuta", Corpo: []byte("{}")})

	if !errors.Is(err, ErrNaoRoteada) {
		t.Fatalf("mensagem sem rota aceita em silêncio: %v", err)
	}
}

func TestIntegracaoPublisher_PermitirSemRota(t *testing.T) {
	conn := conexaoReal(t)
	p := NovoPublisher(conn, PublisherConfig{})
	defer p.Close()

	if err := p.Publicar(context.Background(), Mensagem{Exchange: exchangeDeTeste(t), ChaveRoteamento: "x", Corpo: []byte("{}"), PermitirSemRota: true}); err != nil {
		t.Fatal(err)
	}
}

func TestIntegracaoPublisher_RecuperaCanalFechado(t *testing.T) {
	conn := conexaoReal(t)
	exchange := exchangeDeTeste(t)
	entregas := filaLigada(t, conn, exchange, "k")
	p := NovoPublisher(conn, PublisherConfig{})
	defer p.Close()
	if err := p.Publicar(context.Background(), Mensagem{Exchange: exchange, ChaveRoteamento: "k", Corpo: []byte("1")}); err != nil {
		t.Fatal(err)
	}
	<-entregas

	p.mu.Lock()
	_ = p.canal.Close()
	p.mu.Unlock()

	if err := p.Publicar(context.Background(), Mensagem{Exchange: exchange, ChaveRoteamento: "k", Corpo: []byte("2")}); err != nil {
		t.Fatalf("canal fechado não foi reaberto: %v", err)
	}
	select {
	case d := <-entregas:
		if string(d.Body) != "2" {
			t.Fatalf("corpo %s", d.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("segunda mensagem não chegou")
	}
}

func TestIntegracaoPublisher_ExchangeInexistenteNaoFicaPresa(t *testing.T) {
	conn := conexaoReal(t)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	exchange := exchangeDeTeste(t)
	if err := ch.ExchangeDeclare(exchange, "fanout", false, true, false, false, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	p := NovoPublisher(conn, PublisherConfig{})
	defer p.Close()

	if err := p.Publicar(context.Background(), Mensagem{Exchange: exchange, ChaveRoteamento: "x", Corpo: []byte("{}")}); err == nil {
		t.Fatal("declaração incompatível aceita")
	}
	if err := p.Publicar(context.Background(), Mensagem{Exchange: exchangeDeTeste(t), ChaveRoteamento: "x", Corpo: []byte("{}"), PermitirSemRota: true}); err != nil {
		t.Fatalf("publisher não se recuperou do canal derrubado: %v", err)
	}
}
