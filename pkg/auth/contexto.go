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
	EmpresaId      string
	ColaboracaoId  string
	DepartamentoId string
	CargoId        string
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
