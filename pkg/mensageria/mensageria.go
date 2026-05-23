// Package mensageria fornece um helper de conexão RabbitMQ padronizado
// e utilitários comuns para publishers e consumers.
//
// Importante: este pacote NÃO declara filas/exchanges específicas. Cada
// serviço continua dono dos seus nomes em internal/infra/mq/names.go e
// chama os métodos Declarar* deste pacote durante o startup.
package mensageria

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/rabbitmq/amqp091-go"
)

// Config configura a conexão AMQP.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	VHost    string
}

func (c Config) validar() error {
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("mensageria: Host obrigatório")
	}
	if c.Port <= 0 {
		return errors.New("mensageria: Port inválido")
	}
	if strings.TrimSpace(c.User) == "" {
		return errors.New("mensageria: User obrigatório")
	}
	return nil
}

func (c Config) urlAMQP() string {
	vhost := c.VHost
	if vhost == "" {
		vhost = "/"
	}
	return fmt.Sprintf(
		"amqp://%s:%s@%s:%d%s",
		url.QueryEscape(c.User),
		url.QueryEscape(c.Password),
		c.Host,
		c.Port,
		vhost,
	)
}

// Conectar abre uma conexão AMQP com o RabbitMQ.
func Conectar(cfg Config) (*amqp091.Connection, error) {
	if err := cfg.validar(); err != nil {
		return nil, err
	}
	conn, err := amqp091.Dial(cfg.urlAMQP())
	if err != nil {
		return nil, fmt.Errorf("mensageria: dial: %w", err)
	}
	return conn, nil
}

// DeclararExchange cria uma exchange topic durável no broker. Usado no
// startup de cada serviço para garantir que suas exchanges existem.
func DeclararExchange(conn *amqp091.Connection, nome string) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("mensageria: abrir channel: %w", err)
	}
	defer ch.Close()

	return ch.ExchangeDeclare(
		nome,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
}

// DeclararFila cria uma fila durável e a vincula à exchange por routing key.
// Ideal para consumers que precisam garantir a existência da fila durante
// o startup, em vez de depender de scripts de provisionamento.
func DeclararFila(conn *amqp091.Connection, fila, exchange, routingKey string) (amqp091.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return amqp091.Queue{}, fmt.Errorf("mensageria: abrir channel: %w", err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(fila, true, false, false, false, nil)
	if err != nil {
		return amqp091.Queue{}, fmt.Errorf("mensageria: declarar fila %q: %w", fila, err)
	}
	if exchange != "" {
		if err := ch.QueueBind(q.Name, routingKey, exchange, false, nil); err != nil {
			return amqp091.Queue{}, fmt.Errorf("mensageria: bind %q->%q: %w", fila, exchange, err)
		}
	}
	return q, nil
}
