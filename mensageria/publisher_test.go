package mensageria

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbitmq/amqp091-go"
)

type provedorFalho struct{ err error }

func (p provedorFalho) Channel() (*amqp091.Channel, error) { return nil, p.err }

func TestPublisher_FalhaAoAbrirCanalPropagaCausa(t *testing.T) {
	queda := errors.New("conexão RabbitMQ não disponível")
	p := NovoPublisher(provedorFalho{err: queda}, PublisherConfig{})

	err := p.Publicar(context.Background(), Mensagem{Exchange: "tarefa", ChaveRoteamento: "x", Corpo: []byte("{}")})

	if !errors.Is(err, queda) {
		t.Fatalf("causa perdida: %v", err)
	}
}

func TestPublisher_FechadoRecusaPublicacao(t *testing.T) {
	p := NovoPublisher(provedorFalho{}, PublisherConfig{})
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}

	if err := p.Publicar(context.Background(), Mensagem{Exchange: "tarefa"}); !errors.Is(err, ErrPublisherFechado) {
		t.Fatalf("publicação após Close: %v", err)
	}
}

func TestPublisher_TempoConfirmacaoPadrao(t *testing.T) {
	if p := NovoPublisher(provedorFalho{}, PublisherConfig{}); p.tempo != confirmacaoPadrao {
		t.Fatalf("tempo = %s", p.tempo)
	}
}
