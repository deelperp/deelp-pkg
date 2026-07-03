// Package seguranca reúne middlewares HTTP de proteção da plataforma:
// rate limiting distribuído, bloqueio de IPs após múltiplas falhas e
// auditoria de padrões suspeitos.
//
// Diferenças importantes da versão anterior (autenticacao-service):
//   - RateLimiter agora suporta backend Redis e funciona com múltiplas
//     réplicas do mesmo serviço (estava in-memory antes — bug crítico
//     apontado pelo CLAUDE.md).
//   - X-Forwarded-For tratado corretamente (apenas primeiro IP).
//   - Respostas de erro via Responder injetável (não acopla a out.ResultadoDTO).
package seguranca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Bloqueio struct {
	Sucesso  bool   `json:"sucesso"`
	Mensagem string `json:"mensagem"`
}

// Responder permite ao consumidor injetar seu próprio formato de erro.
type Responder func(w http.ResponseWriter, status int, mensagem string)

func responderPadrao(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Bloqueio{Sucesso: false, Mensagem: msg})
}

// RateLimiterConfig configura o rate limiter.
//
// Limite e Janela são obrigatórios. Quando Redis estiver presente, o
// limiter usa contagem distribuída via INCR+EXPIRE; senão cai em
// implementação in-memory (apenas dev / instância única).
type RateLimiterConfig struct {
	Limite    int
	Janela    time.Duration
	Prefixo   string // chave Redis. Default "rl:"
	Redis     *redis.Client
	Responder Responder
}

// RateLimiter aplica rate limit por IP com backend opcional Redis.
type RateLimiter struct {
	cfg       RateLimiterConfig
	mem       *limiterMemoria
	responder Responder
}

func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	if cfg.Prefixo == "" {
		cfg.Prefixo = "rl:"
	}
	rl := &RateLimiter{
		cfg:       cfg,
		responder: cfg.Responder,
	}
	if rl.responder == nil {
		rl.responder = responderPadrao
	}
	if cfg.Redis == nil {
		rl.mem = newLimiterMemoria(cfg.Limite, cfg.Janela)
	}
	return rl
}

// Allow verifica se a request é permitida. Usa Redis quando configurado;
// caso contrário usa memória local.
func (rl *RateLimiter) Allow(ctx context.Context, ip string) (bool, error) {
	if rl.cfg.Redis != nil {
		return rl.allowRedis(ctx, ip)
	}
	return rl.mem.allow(ip), nil
}

func (rl *RateLimiter) allowRedis(ctx context.Context, ip string) (bool, error) {
	key := rl.cfg.Prefixo + ip
	pipe := rl.cfg.Redis.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, rl.cfg.Janela)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("rate_limiter: redis: %w", err)
	}
	return incr.Val() <= int64(rl.cfg.Limite), nil
}

// Middleware retorna o handler que aplica o rate limit.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := IPDoRequest(r)
		allowed, err := rl.Allow(r.Context(), ip)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		if !allowed {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.cfg.Limite))
			w.Header().Set("Retry-After", rl.cfg.Janela.String())
			rl.responder(w, http.StatusTooManyRequests, "Muitas requisições. Tente novamente mais tarde.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type limiterMemoria struct {
	mu     sync.Mutex
	visits map[string][]time.Time
	limite int
	janela time.Duration
}

func newLimiterMemoria(limite int, janela time.Duration) *limiterMemoria {
	l := &limiterMemoria{
		visits: make(map[string][]time.Time),
		limite: limite,
		janela: janela,
	}
	go l.cleanup()
	return l
}

func (l *limiterMemoria) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-l.janela)
	hist := l.visits[ip]
	novo := hist[:0]
	for _, t := range hist {
		if t.After(cutoff) {
			novo = append(novo, t)
		}
	}
	if len(novo) >= l.limite {
		l.visits[ip] = novo
		return false
	}
	l.visits[ip] = append(novo, now)
	return true
}

func (l *limiterMemoria) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		cutoff := time.Now().Add(-l.janela * 2)
		for ip, hist := range l.visits {
			if len(hist) == 0 || hist[len(hist)-1].Before(cutoff) {
				delete(l.visits, ip)
			}
		}
		l.mu.Unlock()
	}
}
