// Package assinatura gateia operações de escrita conforme a situação do
// contrato do tenant.
//
// Antes disso, contrato expirado era bloqueio só de tela: o frontend abria um
// modal e todo endpoint continuava respondendo normalmente, então bastava
// fechar o modal pelo devtools para seguir usando a plataforma de graça.
package assinatura

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deelperp/deelp-pkg/auth"
)

// MotivoContratoInativo é devolvido no corpo para o cliente distinguir bloqueio
// por pagamento (402) de bloqueio por permissão (403).
const MotivoContratoInativo = "contrato_inativo"

type Erro struct {
	Sucesso  bool   `json:"sucesso"`
	Mensagem string `json:"mensagem"`
	Motivo   string `json:"motivo,omitempty"`
}

// Checker informa se o tenant pode operar. Erro significa "não consegui
// verificar" — é diferente de "não pode", e o middleware trata os dois casos de
// forma distinta.
type Checker interface {
	ContratoAtivo(ctx context.Context, bearer, empresaId string) (ativo bool, motivo string, err error)
}

type Config struct {
	Logger     *slog.Logger
	CookieName string
}

func (c Config) log(msg string, args ...any) {
	if c.Logger != nil {
		c.Logger.Warn(msg, args...)
	}
}

// RequerContratoAtivo bloqueia a requisição quando o contrato do tenant não
// permite operar.
//
// Política de falha deliberadamente ASSIMÉTRICA:
//
//   - resposta explícita de contrato inativo  -> 402, bloqueia;
//   - erro de transporte (serviço fora, timeout) -> DEIXA PASSAR.
//
// Fail-closed aqui derrubaria escrita em toda a plataforma sempre que o
// cliente-service piscasse. O prejuízo de deixar passar é uma janela curta de
// uso por quem devia estar bloqueado; o de fechar é parar todos os clientes
// adimplentes. Por isso o erro é logado — a janela precisa ser visível, não
// silenciosa.
//
// Deve rodar DEPOIS de auth.Autenticacao: sem claims não há tenant para
// verificar, e nesse caso a requisição segue (quem barra token ausente é o
// Autenticacao, não este middleware).
func RequerContratoAtivo(cfg Config, checker Checker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			empresaId, ok := auth.EmpresaIdClaimString(r.Context())
			if !ok || empresaId == "" {
				next.ServeHTTP(w, r)
				return
			}

			if checker == nil {
				cfg.log("assinatura: checker não configurado, requisição liberada", "empresaId", empresaId)
				next.ServeHTTP(w, r)
				return
			}

			ativo, motivo, err := checker.ContratoAtivo(r.Context(), bearerDoRequest(r, cfg.CookieName), empresaId)
			if err != nil {
				cfg.log("assinatura: falha ao verificar contrato, requisição liberada",
					"empresaId", empresaId, "erro", err)
				next.ServeHTTP(w, r)
				return
			}

			if !ativo {
				responder(w, motivo)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func bearerDoRequest(r *http.Request, cookieName string) string {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw != "" {
		return raw
	}
	if cookieName != "" {
		if c, err := r.Cookie(cookieName); err == nil {
			return strings.TrimSpace(c.Value)
		}
	}
	return ""
}

func responder(w http.ResponseWriter, situacao string) {
	mensagem := "O contrato da sua empresa não está ativo. Regularize a assinatura para continuar."
	if situacao == "expirado" {
		mensagem = "O período contratado terminou. Escolha um plano para continuar usando a plataforma."
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(Erro{
		Sucesso:  false,
		Mensagem: mensagem,
		Motivo:   MotivoContratoInativo,
	})
}

// ApenasEscrita aplica o gate somente a métodos que alteram estado.
//
// Leitura precisa continuar aberta: o cliente bloqueado tem que conseguir ver
// os próprios dados e chegar até a tela de pagamento. Bloquear GET deixaria a
// plataforma inutilizável justamente para quem quer voltar a pagar.
func ApenasEscrita(middleware func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		comGate := middleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
			default:
				comGate.ServeHTTP(w, r)
			}
		})
	}
}
