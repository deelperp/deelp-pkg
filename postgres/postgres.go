// Package postgres fornece um helper de conexão padronizado com PostgreSQL.
//
// Substitui implementações ad-hoc com tunings divergentes:
//   - ordem-service: sem tuning de pool (usava o default do database/sql,
//     2 conexões idle, o que provocava churn nos endpoints concorrentes)
//   - estoque-service: pool tunado (50 abertas, 20 idle, 30 min lifetime)
//
// Esta versão adota o tuning do estoque-service como default e expõe overrides
// via Config para serviços com perfil de carga diferente.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Config configura a conexão.
//
// Host, Port, User, Password, Name são obrigatórios.
//
// SSLMode aceita disable | require | verify-ca | verify-full.
// Default é "disable" (mantém comportamento atual), mas para produção
// deve ser pelo menos "require".
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string

	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func (c Config) validar() error {
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("postgres: Host obrigatório")
	}
	if c.Port <= 0 {
		return errors.New("postgres: Port inválido")
	}
	if strings.TrimSpace(c.User) == "" {
		return errors.New("postgres: User obrigatório")
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("postgres: Name obrigatório")
	}
	return nil
}

// Conectar abre uma conexão *sql.DB e valida com Ping.
//
// O context controla apenas o Ping inicial — o pool de conexões herda
// timeouts via Conn.QueryContext/ExecContext do consumidor.
func Conectar(ctx context.Context, cfg Config) (*sql.DB, error) {
	if err := cfg.validar(); err != nil {
		return nil, err
	}

	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, sslMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: abrir conexão: %w", err)
	}

	db.SetMaxOpenConns(ouPadrao(cfg.MaxOpenConns, 50))
	db.SetMaxIdleConns(ouPadrao(cfg.MaxIdleConns, 20))
	db.SetConnMaxLifetime(ouPadraoDuration(cfg.ConnMaxLifetime, 30*time.Minute))
	db.SetConnMaxIdleTime(ouPadraoDuration(cfg.ConnMaxIdleTime, 5*time.Minute))

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: ping falhou: %w", err)
	}
	return db, nil
}

func ouPadrao(v, padrao int) int {
	if v <= 0 {
		return padrao
	}
	return v
}

func ouPadraoDuration(v, padrao time.Duration) time.Duration {
	if v <= 0 {
		return padrao
	}
	return v
}
