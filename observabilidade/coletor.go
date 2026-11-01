package observabilidade

import (
	"context"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// EnvHabilitada desliga a observabilidade inteira quando vale false/0/off/nao.
// Vazio ou qualquer outro valor mantém o modo automático: o serviço sonda o
// Collector e só passa a exportar quando ele responde.
const EnvHabilitada = "OBSERVABILIDADE_HABILITADA"

var (
	intervaloSondagem  = 30 * time.Second
	timeoutSondagem    = 500 * time.Millisecond
	intervaloAvisoErro = 5 * time.Minute
)

// Habilitada informa se a observabilidade não foi desligada pelo ambiente.
func Habilitada() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvHabilitada))) {
	case "false", "0", "off", "nao", "não", "no", "desabilitada":
		return false
	}
	return true
}

func (c Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

func desligarNada(context.Context) error { return nil }

func (c Config) enderecoSondagem(e endpointConfig) string {
	if _, _, err := net.SplitHostPort(e.host); err == nil {
		return e.host
	}
	porta := "4318"
	switch {
	case e.secure:
		porta = "443"
	case c.protocoloEfetivo() == ProtocoloGRPC:
		porta = "4317"
	}
	return net.JoinHostPort(strings.Trim(e.host, "[]"), porta)
}

func coletorAcessivel(endereco string) bool {
	conn, err := net.DialTimeout("tcp", endereco, timeoutSondagem)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func aguardarColetor(cfg Config, endpoint endpointConfig, endereco string) Desligar {
	var (
		mu       sync.Mutex
		desligar Desligar = desligarNada
	)
	parar := make(chan struct{})
	terminou := make(chan struct{})

	go func() {
		defer close(terminou)
		ticker := time.NewTicker(intervaloSondagem)
		defer ticker.Stop()
		for {
			select {
			case <-parar:
				return
			case <-ticker.C:
				if !coletorAcessivel(endereco) {
					continue
				}
				d, err := instalarProviders(context.Background(), cfg, endpoint)
				if err != nil {
					cfg.logger().Warn("observabilidade: collector respondeu mas os exporters falharam", "erro", err)
					return
				}
				mu.Lock()
				desligar = d
				mu.Unlock()
				cfg.logger().Info("observabilidade ativada: collector disponível", "servico", cfg.NomeServico, "endpoint", cfg.Endpoint)
				return
			}
		}
	}()

	var once sync.Once
	return func(c context.Context) error {
		once.Do(func() { close(parar) })
		<-terminou
		mu.Lock()
		defer mu.Unlock()
		return desligar(c)
	}
}

// errorHandlerLimitado evita que um Collector fora do ar encha o terminal: o
// SDK reporta cada exportação falha, a cada poucos segundos, para sempre.
type errorHandlerLimitado struct {
	mu         sync.Mutex
	logger     *slog.Logger
	ultimo     time.Time
	suprimidos int
}

func (h *errorHandlerLimitado) Handle(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	agora := time.Now()
	if !h.ultimo.IsZero() && agora.Sub(h.ultimo) < intervaloAvisoErro {
		h.suprimidos++
		return
	}
	h.logger.Warn("observabilidade: falha ao exportar telemetria", "erro", err, "suprimidos", h.suprimidos, "proximoAvisoEm", intervaloAvisoErro.String())
	h.ultimo = agora
	h.suprimidos = 0
}
