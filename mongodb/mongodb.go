// Package mongodb fornece um helper de conexão padronizado com MongoDB.
//
// Substitui implementações duplicadas em ordem-service e tarefa-service.
package mongodb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Config configura a conexão.
//
// Há dois modos de configuração mutuamente exclusivos:
//
//  1. URI completa — quando URI != "" o pkg usa esse valor direto (útil para
//     replicasets ou setups com parâmetros customizados). Name continua sendo
//     necessário para indicar o database de destino.
//  2. Host/Port/User/Password — quando URI == "" o pkg monta a URI a partir
//     desses campos. AuthSource default "admin"; AuthMechanism default
//     "SCRAM-SHA-256" (use "auto" para deixar o driver negociar).
type Config struct {
	URI           string
	Host          string
	Port          int
	User          string
	Password      string
	Name          string
	AuthSource    string
	AuthMechanism string

	MaxPoolSize            uint64
	MinPoolSize            uint64
	MaxConnIdleTime        time.Duration
	ServerSelectionTimeout time.Duration
	ConnectTimeout         time.Duration
}

func (c Config) validar() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("mongodb: Name (database) obrigatório")
	}
	if strings.TrimSpace(c.URI) != "" {
		return nil
	}
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("mongodb: Host obrigatório (ou forneça URI)")
	}
	if c.Port <= 0 {
		return errors.New("mongodb: Port inválido")
	}
	return nil
}

func (c Config) uri() string {
	if u := strings.TrimSpace(c.URI); u != "" {
		return u
	}
	if c.User != "" && c.Password != "" {
		authDB := c.AuthSource
		if authDB == "" {
			authDB = "admin"
		}
		base := fmt.Sprintf(
			"mongodb://%s:%s@%s:%d/?authSource=%s",
			url.QueryEscape(c.User),
			url.QueryEscape(c.Password),
			c.Host,
			c.Port,
			url.QueryEscape(authDB),
		)
		mech := strings.TrimSpace(c.AuthMechanism)
		if mech == "" {
			mech = "SCRAM-SHA-256"
		}
		if strings.EqualFold(mech, "auto") {
			return base
		}
		return base + "&authMechanism=" + url.QueryEscape(mech)
	}
	return fmt.Sprintf("mongodb://%s:%d", c.Host, c.Port)
}

// Conectar abre uma conexão Mongo e valida com Ping. Retorna o *Database
// já apontado para cfg.Name.
func Conectar(ctx context.Context, cfg Config) (*mongo.Database, error) {
	if err := cfg.validar(); err != nil {
		return nil, err
	}

	ctxConn, cancel := context.WithTimeout(ctx, ouPadrao(cfg.ConnectTimeout, 10*time.Second))
	defer cancel()

	opts := options.Client().
		ApplyURI(cfg.uri()).
		SetMaxPoolSize(ouPadraoUint(cfg.MaxPoolSize, 100)).
		SetMinPoolSize(ouPadraoUint(cfg.MinPoolSize, 10)).
		SetMaxConnIdleTime(ouPadrao(cfg.MaxConnIdleTime, 30*time.Second)).
		SetServerSelectionTimeout(ouPadrao(cfg.ServerSelectionTimeout, 5*time.Second)).
		SetConnectTimeout(ouPadrao(cfg.ConnectTimeout, 10*time.Second))

	client, err := mongo.Connect(ctxConn, opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb: conectar: %w", err)
	}
	if err := client.Ping(ctxConn, nil); err != nil {
		_ = client.Disconnect(ctxConn)
		return nil, fmt.Errorf("mongodb: ping falhou: %w", err)
	}
	return client.Database(cfg.Name), nil
}

func ouPadrao(v, padrao time.Duration) time.Duration {
	if v <= 0 {
		return padrao
	}
	return v
}

func ouPadraoUint(v, padrao uint64) uint64 {
	if v == 0 {
		return padrao
	}
	return v
}
