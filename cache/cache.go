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
	"encoding/json"
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

// ErrCacheMiss é retornado por CacheService.Get quando a chave não existe.
var ErrCacheMiss = fmt.Errorf("cache: chave não encontrada")

// CacheService oferece operações Get/Set/Delete sobre um *redis.Client
// obtido via Conectar. Centralizado no pkg para que todos os microserviços
// usem a mesma implementação.
type CacheService struct {
	client *redis.Client
	ttl    time.Duration
}

// NewCacheService cria um CacheService com TTL padrão para operações Set.
func NewCacheService(client *redis.Client, ttl time.Duration) *CacheService {
	return &CacheService{client: client, ttl: ttl}
}

// Get recupera e desserializa JSON. Retorna ErrCacheMiss se a chave não existe.
func (c *CacheService) Get(ctx context.Context, key string, dest interface{}) error {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return ErrCacheMiss
	}
	if err != nil {
		return fmt.Errorf("cache: get %q: %w", key, err)
	}
	return json.Unmarshal([]byte(val), dest)
}

// Set serializa e armazena com o TTL padrão configurado.
func (c *CacheService) Set(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: marshal %q: %w", key, err)
	}
	return c.client.Set(ctx, key, data, c.ttl).Err()
}

// SetWithTTL serializa e armazena com TTL explícito.
func (c *CacheService) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: marshal %q: %w", key, err)
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

// Delete remove uma chave.
func (c *CacheService) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// DeletePattern remove todas as chaves que correspondem ao padrão glob.
// Atenção: KEYS bloqueia o Redis em bases grandes — use apenas em bases pequenas
// ou durante janelas de manutenção. Para produção com volume alto, prefira SCAN.
func (c *CacheService) DeletePattern(ctx context.Context, pattern string) error {
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("cache: keys %q: %w", pattern, err)
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}
	return nil
}

// Exists verifica se uma chave existe.
func (c *CacheService) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
