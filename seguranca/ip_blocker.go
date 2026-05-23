package seguranca

import (
	"net/http"
	"sync"
	"time"
)

// IPBlockerConfig configura o bloqueio de IPs após múltiplas falhas.
type IPBlockerConfig struct {
	MaxTentativas    int           // tentativas antes de bloquear
	DuracaoBloqueio  time.Duration // por quanto tempo bloqueia
	JanelaTentativas time.Duration // janela para contar tentativas
	Responder        Responder
}

// IPBlocker bloqueia IPs após múltiplas falhas consecutivas.
// Implementação in-memory por enquanto — para múltiplas réplicas, evoluir
// para Redis (mesmo padrão do RateLimiter).
type IPBlocker struct {
	cfg       IPBlockerConfig
	mu        sync.Mutex
	bloqueios map[string]time.Time
	falhas    map[string]int
	responder Responder
}

func NewIPBlocker(cfg IPBlockerConfig) *IPBlocker {
	b := &IPBlocker{
		cfg:       cfg,
		bloqueios: make(map[string]time.Time),
		falhas:    make(map[string]int),
		responder: cfg.Responder,
	}
	if b.responder == nil {
		b.responder = responderPadrao
	}
	go b.cleanup()
	return b
}

// RegistrarFalha incrementa o contador. Bloqueia o IP se atingir o máximo.
func (b *IPBlocker) RegistrarFalha(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.falhas[ip]++
	if b.falhas[ip] >= b.cfg.MaxTentativas {
		b.bloqueios[ip] = time.Now()
		delete(b.falhas, ip)
	}
}

// RegistrarSucesso reseta o contador de falhas para o IP.
func (b *IPBlocker) RegistrarSucesso(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.falhas, ip)
}

// EstaBloqueado verifica e expira o bloqueio se já passou da duração.
func (b *IPBlocker) EstaBloqueado(ip string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	bt, ok := b.bloqueios[ip]
	if !ok {
		return false
	}
	if time.Since(bt) > b.cfg.DuracaoBloqueio {
		delete(b.bloqueios, ip)
		return false
	}
	return true
}

// Restante devolve o tempo de bloqueio que ainda falta para o IP.
func (b *IPBlocker) Restante(ip string) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	bt, ok := b.bloqueios[ip]
	if !ok {
		return 0
	}
	r := b.cfg.DuracaoBloqueio - time.Since(bt)
	if r < 0 {
		return 0
	}
	return r
}

// Middleware retorna o handler HTTP.
func (b *IPBlocker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := IPDoRequest(r)
		if b.EstaBloqueado(ip) {
			w.Header().Set("Retry-After", b.Restante(ip).String())
			b.responder(w, http.StatusForbidden, "IP bloqueado devido a múltiplas tentativas falhas.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *IPBlocker) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		b.mu.Lock()
		now := time.Now()
		for ip, t := range b.bloqueios {
			if now.Sub(t) > b.cfg.DuracaoBloqueio {
				delete(b.bloqueios, ip)
			}
		}
		b.mu.Unlock()
	}
}
