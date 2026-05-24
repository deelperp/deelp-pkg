package seguranca

import (
	"errors"
	"strings"
)

// ValidarConfigProducao garante que configurações sensíveis estão presentes
// quando o serviço sobe com PRODUCTION=true. Falha rápido evita deploys em
// produção com SECRET_KEY vazia, CORS curinga ou outras configurações que
// dependem do ambiente.
//
// Cada serviço deve chamar esta função no final do seu Load(), passando os
// valores carregados a partir de envconfig.
type ConfigProducao struct {
	Production bool
	SecretKey  string
	CORSOrigin string
}

func ValidarConfigProducao(c ConfigProducao) error {
	if !c.Production {
		return nil
	}
	if strings.TrimSpace(c.SecretKey) == "" {
		return errors.New("SECRET_KEY é obrigatório em produção")
	}
	origin := strings.TrimSpace(c.CORSOrigin)
	if origin == "" || origin == "*" {
		return errors.New("CORS_ORIGIN deve ser uma origem explícita em produção (curinga não é permitido)")
	}
	return nil
}
