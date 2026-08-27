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
		if strings.HasPrefix(caminho, "/relatorio-service/v1/relatorios/") {
			return !strings.HasSuffix(caminho, "/solicitar")
		}
	}
	return false
}

// Cada sufixo corresponde a rotas reais e verificadas. Sufixo genérico não
// entra: adotaria sozinho a próxima rota que alguém criar.
var sufixosDeLeitura = []string{
	"/pesquisar",
	"/consultar",
	"/listar",
	"/metricas",
	"/metricas-integradas",
	"/previa-calculo",
	"/gerar-pdf-solicitacao",
}

var caminhosDeLeitura = []string{
	"/notificacao-service/v1",
}
