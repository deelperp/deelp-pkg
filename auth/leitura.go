package auth

import (
	"net/http"
	"strings"
)

func EhRequisicaoDeLeitura(r *http.Request) bool {
	if r == nil {
		return false
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	case http.MethodPost:
		caminho := strings.TrimRight(r.URL.Path, "/")
		for _, sufixo := range sufixosDeLeitura {
			if strings.HasSuffix(caminho, sufixo) {
				return true
			}
		}
		for _, exacto := range caminhosDeLeitura {
			if caminho == exacto {
				return true
			}
		}
		if strings.Contains(caminho, "/relatorios/") || strings.HasSuffix(caminho, "/relatorios") {
			return true
		}
	}
	return false
}

var sufixosDeLeitura = []string{
	"/pesquisar",
	"/consultar",
	"/listar",
	"/metricas",
	"/metricas-integradas",
	"/previa-calculo",
	"/danfe",
	"/damdfe",
	"/proposta",
	"/comparativo",
	"/gerar-pdf-solicitacao",
	"/pdf",
}

var caminhosDeLeitura = []string{
	"/notificacao-service/v1",
}
