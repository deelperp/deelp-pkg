// Package auth fornece o middleware HTTP de autenticação JWT e tenant guard
// compartilhado entre todos os microserviços da plataforma Deelp.
//
// O pacote não importa tipos específicos de nenhum serviço. Quem usa pode
// (opcionalmente) injetar um Responder próprio em Config para customizar o
// formato das respostas de erro, mas o formato padrão {sucesso, mensagem}
// já cobre o esperado pelo app Flutter e pelo frontend web.
package auth

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const (
	chaveClaims         contextKey = "deelp.auth.claims"
	chaveUsuarioId      contextKey = "deelp.auth.usuarioId"
	chaveEmpresaId      contextKey = "deelp.auth.empresaId"
	chaveColaboracaoId  contextKey = "deelp.auth.colaboracaoId"
	chaveDepartamentoId contextKey = "deelp.auth.departamentoId"
	chaveCargoId        contextKey = "deelp.auth.cargoId"
)

// Claims representa os campos extraídos do JWT. Apenas campos string são
// expostos — qualquer claim numérico/temporal (exp, iat) é validado pela
// biblioteca jwt durante o ParseWithClaims.
type Claims struct {
	UsuarioId      string
	Email          string
	Nome           string
	Sobrenome      string
	EmpresaId      string
	ColaboracaoId  string
	DepartamentoId string
	CargoId        string
	// Emitido por autenticacao-service a partir de PLATFORM_ADMIN_EMAILS.
	// Habilita as telas de operação da própria Deelp, que agem sobre outros
	// tenants e por isso ficam fora do recorte por empresaId.
	IsPlatformAdmin bool
	// Recorte fino do painel: quais abas este operador opera. Ver
	// ModulosPlataformaDisponiveis no autenticacao-service.
	PlataformaModulos []string
	SuporteEmpresaId  string
	// Sessão que originou o token. O consentimento é conferido por ela, não pelo exp.
	SessaoSuporteId string
	// Unix do fim da elevação de escrita. Zero significa somente leitura.
	SuporteEscritaAte int64
}

// EhPlatformAdminDoContexto informa se o token é de um operador da plataforma.
func EhPlatformAdminDoContexto(ctx context.Context) bool {
	c, ok := ClaimsDoContexto(ctx)
	return ok && c.IsPlatformAdmin
}

// PodeNoModuloDePlataforma é o gate por aba do painel. Fail-closed: sem a claim
// de operador, ou sem o módulo na lista, recusa. É o que impede que quem presta
// suporte enxergue a receita da empresa.
func PodeNoModuloDePlataforma(ctx context.Context, modulo string) bool {
	c, ok := ClaimsDoContexto(ctx)
	if !ok || !c.IsPlatformAdmin {
		return false
	}
	for _, m := range c.PlataformaModulos {
		if strings.EqualFold(strings.TrimSpace(m), modulo) {
			return true
		}
	}
	return false
}

func EhSessaoSuporte(ctx context.Context) bool {
	c, ok := ClaimsDoContexto(ctx)
	return ok && c.SuporteEmpresaId != ""
}

func SuporteEmpresaIdDoContexto(ctx context.Context) (uuid.UUID, bool) {
	c, ok := ClaimsDoContexto(ctx)
	if !ok || c.SuporteEmpresaId == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(c.SuporteEmpresaId)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// SuportePodeEscrever informa se a elevação de escrita ainda está na janela.
func SuportePodeEscrever(c Claims, agora time.Time) bool {
	return c.SuporteEscritaAte > 0 && agora.Unix() < c.SuporteEscritaAte
}

// SessaoSuporteIdDoContexto devolve a sessão de suporte que originou o token.
func SessaoSuporteIdDoContexto(ctx context.Context) (uuid.UUID, bool) {
	c, ok := ClaimsDoContexto(ctx)
	if !ok || c.SessaoSuporteId == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(c.SessaoSuporteId)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// ComClaims devolve um context.Context com as claims injetadas. Use para
// propagar a identidade do request até o repositório/use case.
func ComClaims(ctx context.Context, c Claims) context.Context {
	ctx = context.WithValue(ctx, chaveClaims, c)
	ctx = context.WithValue(ctx, chaveUsuarioId, c.UsuarioId)
	ctx = context.WithValue(ctx, chaveEmpresaId, c.EmpresaId)
	ctx = context.WithValue(ctx, chaveColaboracaoId, c.ColaboracaoId)
	ctx = context.WithValue(ctx, chaveDepartamentoId, c.DepartamentoId)
	ctx = context.WithValue(ctx, chaveCargoId, c.CargoId)
	return ctx
}

func ClaimsDoContexto(ctx context.Context) (Claims, bool) {
	v, ok := ctx.Value(chaveClaims).(Claims)
	return v, ok
}

// EmailDoContexto devolve o e-mail do usuário autenticado a partir das claims
// injetadas pelo middleware. Retorna ("", false) quando não há claim.
func EmailDoContexto(ctx context.Context) (string, bool) {
	c, ok := ClaimsDoContexto(ctx)
	if !ok || c.Email == "" {
		return "", false
	}
	return c.Email, true
}

// NomeCompletoDoContexto devolve "Nome Sobrenome" do JWT (fallback: e-mail).
func NomeCompletoDoContexto(ctx context.Context) (string, bool) {
	c, ok := ClaimsDoContexto(ctx)
	if !ok {
		return "", false
	}
	nome := strings.TrimSpace(strings.TrimSpace(c.Nome) + " " + strings.TrimSpace(c.Sobrenome))
	if nome == "" {
		nome = strings.TrimSpace(c.Email)
	}
	if nome == "" {
		return "", false
	}
	return nome, true
}

func UsuarioIdDoContexto(ctx context.Context) (uuid.UUID, bool) {
	s, _ := ctx.Value(chaveUsuarioId).(string)
	if s == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func EmpresaIdDoContexto(ctx context.Context) (uuid.UUID, bool) {
	s, _ := ctx.Value(chaveEmpresaId).(string)
	if s == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// EmpresaIdClaimString devolve a string crua do claim (útil para
// cross-checks de rotas legacy que recebem empresaId no body).
func EmpresaIdClaimString(ctx context.Context) (string, bool) {
	s, _ := ctx.Value(chaveEmpresaId).(string)
	if s == "" {
		return "", false
	}
	return s, true
}

func DepartamentoIdDoContexto(ctx context.Context) (string, bool) {
	s, _ := ctx.Value(chaveDepartamentoId).(string)
	if s == "" {
		return "", false
	}
	return s, true
}

func ColaboracaoIdDoContexto(ctx context.Context) (string, bool) {
	s, _ := ctx.Value(chaveColaboracaoId).(string)
	if s == "" {
		return "", false
	}
	return s, true
}

func CargoIdDoContexto(ctx context.Context) (string, bool) {
	s, _ := ctx.Value(chaveCargoId).(string)
	if s == "" {
		return "", false
	}
	return s, true
}
