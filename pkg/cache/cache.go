// Package cache fornece um cliente Redis padronizado para todos os
// microserviços da plataforma Deelp.
//
// Substitui as implementações ad-hoc que existiam em
// estoque-service/internal/infra/caching/redis e
// relatorio-service/internal/infra/database/redis. Resolve a inconsistência
// de versão go-redis v9.5.1 vs v9.18.0 que o CLAUDE.md apontava.
//
// Importante: o uso de Redis para state de servidor (rate limiter,
// sessão, etc.) também depende deste pacote — não duplicar conexões.
package cache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config configura a conexão Redis. Todos os campos são obrigatórios
// exceto Password (default vazio) e DB (default 0).
type Config struct {
	Host     string
	Port     int
	Password string
	DB       int

	// Timeouts. Quando zero, usa defaults conservadores. Em produção
	// recomenda-se valores explícitos por serviço.
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	PoolSize        int
	MinIdleConns    int
	PoolTimeout     time.Duration
	ConnMaxIdleTime time.Duration
}

func (c Config) validar() error {
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("cache: Host obrigatório")
	}
	if c.Port <= 0 {
		return errors.New("cache: Port inválido")
	}
	return nil
}

// Conectar abre uma nova conexão Redis e valida via PING. Retorna erro
// imediato em vez de conexão zumbi se o servidor estiver inacessível.
func Conectar(ctx context.Context, cfg Config) (*redis.Client, error) {
	if err := cfg.validar(); err != nil {
		return nil, err
	}

	opts := &redis.Options{
		Addr:            fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:        cfg.Password,
		DB:              cfg.DB,
		DialTimeout:     ouPadrao(cfg.DialTimeout, 500*time.Millisecond),
		ReadTimeout:     ouPadrao(cfg.ReadTimeout, 300*time.Millisecond),
		WriteTimeout:    ouPadrao(cfg.WriteTimeout, 400*time.Millisecond),
		PoolSize:        ouPadraoInt(cfg.PoolSize, 30),
		MinIdleConns:    ouPadraoInt(cfg.MinIdleConns, 3),
		PoolTimeout:     ouPadrao(cfg.PoolTimeout, 500*time.Millisecond),
		ConnMaxIdleTime: ouPadrao(cfg.ConnMaxIdleTime, 5*time.Minute),
	}

	cli := redis.NewClient(opts)

	ctxPing, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := cli.Ping(ctxPing).Err(); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("cache: ping falhou: %w", err)
	}
	return cli, nil
}

func ouPadrao(v, padrao time.Duration) time.Duration {
	if v <= 0 {
		return padrao
	}
	return v
}

func ouPadraoInt(v, padrao int) int {
	if v <= 0 {
		return padrao
	}
	return v
}
