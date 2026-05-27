package mensageria

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

const (
	backoffInicial = 2 * time.Second
	backoffMaximo  = 60 * time.Second
	fatorMultiplic = 2
)

// GerenciadorConexao mantém uma conexão AMQP ativa com reconexão automática
// via backoff exponencial. Todos os serviços devem preferir este gerenciador
// em vez de armazenar *amqp091.Connection diretamente.
type GerenciadorConexao struct {
	cfg    Config
	mu     sync.RWMutex
	conn   *amqp091.Connection
	onConn []func(*amqp091.Connection)
}

// NovoGerenciadorConexao cria o gerenciador e abre a primeira conexão.
func NovoGerenciadorConexao(cfg Config) (*GerenciadorConexao, error) {
	g := &GerenciadorConexao{cfg: cfg}
	if err := g.conectar(); err != nil {
		return nil, err
	}
	return g, nil
}

// Conexao retorna a conexão ativa atual (thread-safe).
func (g *GerenciadorConexao) Conexao() *amqp091.Connection {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.conn
}

// Channel abre um canal na conexão ativa.
// Satisfaz a interface provedorConexao dos adapters de mensageria.
func (g *GerenciadorConexao) Channel() (*amqp091.Channel, error) {
	g.mu.RLock()
	conn := g.conn
	g.mu.RUnlock()
	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("conexão RabbitMQ não disponível")
	}
	return conn.Channel()
}

// NovoCanal é um alias semântico de Channel para uso interno.
func (g *GerenciadorConexao) NovoCanal() (*amqp091.Channel, error) {
	return g.Channel()
}

// OnReconexao registra um callback chamado sempre que a conexão é restabelecida.
// Use para re-inscrever consumers após queda do broker.
func (g *GerenciadorConexao) OnReconexao(fn func(*amqp091.Connection)) {
	g.mu.Lock()
	g.onConn = append(g.onConn, fn)
	g.mu.Unlock()
}

// IniciarMonitoramento inicia a goroutine que observa NotifyClose e reconecta.
// Bloqueia até ctx ser cancelado — chame em uma goroutine separada.
func (g *GerenciadorConexao) IniciarMonitoramento(ctx context.Context) {
	for {
		g.mu.RLock()
		conn := g.conn
		g.mu.RUnlock()

		notifyClosed := conn.NotifyClose(make(chan *amqp091.Error, 1))

		select {
		case <-ctx.Done():
			return
		case amqpErr, ok := <-notifyClosed:
			if !ok {
				return
			}
			if amqpErr != nil {
				log.Printf("[mensageria] conexão perdida: %v — reconectando...", amqpErr)
			}
		}

		backoff := backoffInicial
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			if err := g.conectar(); err != nil {
				log.Printf("[mensageria] falha ao reconectar: %v — próxima tentativa em %s", err, backoff)
				backoff = min(backoff*fatorMultiplic, backoffMaximo)
				continue
			}

			log.Println("[mensageria] reconectado ao RabbitMQ")
			g.notificarReconexao()
			break
		}
	}
}

func (g *GerenciadorConexao) conectar() error {
	conn, err := Conectar(g.cfg)
	if err != nil {
		return err
	}
	g.mu.Lock()
	if g.conn != nil && !g.conn.IsClosed() {
		_ = g.conn.Close()
	}
	g.conn = conn
	g.mu.Unlock()
	return nil
}

func (g *GerenciadorConexao) notificarReconexao() {
	g.mu.RLock()
	conn := g.conn
	callbacks := make([]func(*amqp091.Connection), len(g.onConn))
	copy(callbacks, g.onConn)
	g.mu.RUnlock()

	for _, fn := range callbacks {
		fn(conn)
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
