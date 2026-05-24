package seguranca

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// IPBlockerConfig configura o bloqueio de IPs após múltiplas falhas.
type IPBlockerConfig struct {
	MaxTentativas    int
	DuracaoBloqueio  time.Duration
	JanelaTentativas time.Duration
	Responder        Responder
	Redis            *redis.Client
	Prefixo          string
}

// IPBlocker bloqueia IPs após múltiplas falhas consecutivas.
// Quando Config.Redis está preenchido, usa Redis para compartilhar estado
// entre réplicas. Caso contrário, cai em fallback in-memory (apenas para
// desenvolvimento ou deploy com instância única).
type IPBlocker struct {
	cfg       IPBlockerConfig
	mu        sync.Mutex
	bloqueios map[string]time.Time
	falhas    map[string]int
	responder Responder
	prefixo   string
}

func NewIPBlocker(cfg IPBlockerConfig) *IPBlocker {
	prefixo := cfg.Prefixo
	if prefixo == "" {
		prefixo = "ipb:"
	}
	b := &IPBlocker{
		cfg:       cfg,
		bloqueios: make(map[string]time.Time),
		falhas:    make(map[string]int),
		responder: cfg.Responder,
		prefixo:   prefixo,
	}
	if b.responder == nil {
		b.responder = responderPadrao
	}
	if b.cfg.Redis == nil {
		go b.cleanup()
	}
	return b
}

func (b *IPBlocker) chaveFalha(ip string) string  { return b.prefixo + "f:" + ip }
func (b *IPBlocker) chaveBloq(ip string) string   { return b.prefixo + "b:" + ip }

// RegistrarFalha incrementa o contador. Bloqueia o IP se atingir o máximo.
func (b *IPBlocker) RegistrarFalha(ip string) {
	if b.cfg.Redis != nil {
		b.registrarFalhaRedis(ip)
		return
	}
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
	if b.cfg.Redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = b.cfg.Redis.Del(ctx, b.chaveFalha(ip)).Err()
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.falhas, ip)
}

// EstaBloqueado verifica e expira o bloqueio se já passou da duração.
func (b *IPBlocker) EstaBloqueado(ip string) bool {
	if b.cfg.Redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		n, err := b.cfg.Redis.Exists(ctx, b.chaveBloq(ip)).Result()
		if err != nil {
			return false
		}
		return n > 0
	}
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
	if b.cfg.Redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		ttl, err := b.cfg.Redis.TTL(ctx, b.chaveBloq(ip)).Result()
		if err != nil || ttl < 0 {
			return 0
		}
		return ttl
	}
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

func (b *IPBlocker) registrarFalhaRedis(ip string) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	keyF := b.chaveFalha(ip)
	count, err := b.cfg.Redis.Incr(ctx, keyF).Result()
	if err != nil {
		return
	}
	if count == 1 {
		_ = b.cfg.Redis.Expire(ctx, keyF, b.cfg.JanelaTentativas).Err()
	}
	if int(count) >= b.cfg.MaxTentativas {
		_ = b.cfg.Redis.Set(ctx, b.chaveBloq(ip), strconv.FormatInt(count, 10), b.cfg.DuracaoBloqueio).Err()
		_ = b.cfg.Redis.Del(ctx, keyF).Err()
	}
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
