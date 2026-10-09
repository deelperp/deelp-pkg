package transporte

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/deelperp/deelp-pkg/internalauth"
	"github.com/deelperp/deelp-pkg/observabilidade"
	"github.com/deelperp/deelp-pkg/tenantdados/core"
	"github.com/google/uuid"
)

const (
	CaminhoExportacao = "/interno/tenant/exportacao"
	CaminhoExpurgo    = "/interno/tenant/expurgo"

	HeaderEmpresa = "X-Empresa-Id"
)

func responderErro(w http.ResponseWriter, status int, mensagem string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"sucesso": false, "mensagem": mensagem})
}

// empresaDaChamada é fail-closed: sem chave interna válida ou sem X-Empresa-Id
// a rota não responde dado de tenant nenhum.
func empresaDaChamada(w http.ResponseWriter, r *http.Request, v *internalauth.Verificador) (uuid.UUID, bool) {
	if !v.Valida(r) {
		responderErro(w, http.StatusForbidden, "Acesso interno não autorizado")
		return uuid.Nil, false
	}
	empresaID, err := uuid.Parse(r.Header.Get(HeaderEmpresa))
	if err != nil || empresaID == uuid.Nil {
		responderErro(w, http.StatusBadRequest, "X-Empresa-Id obrigatório")
		return uuid.Nil, false
	}
	return empresaID, true
}

// HandlerExportacao devolve o ZIP com os CSV (e anexos) do serviço.
func HandlerExportacao(s core.Servico, v *internalauth.Verificador) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		empresaID, ok := empresaDaChamada(w, r, v)
		if !ok {
			return
		}
		arquivos, err := s.Exportar(r.Context(), empresaID)
		if err != nil {
			responderErro(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_ = core.EscreverZip(w, arquivos)
	}
}

// HandlerExpurgo: POST ?fase=1|2&simular=true|false. Sem simular=false
// explícito, só conta — o padrão nunca apaga.
func HandlerExpurgo(s core.Servico, v *internalauth.Verificador) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		empresaID, ok := empresaDaChamada(w, r, v)
		if !ok {
			return
		}
		fase, _ := strconv.Atoi(r.URL.Query().Get("fase"))
		simular := r.URL.Query().Get("simular") != "false"
		relatorio, err := s.Expurgar(r.Context(), empresaID, fase, simular)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"sucesso": false, "mensagem": err.Error(), "conteudo": relatorio})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sucesso": true, "conteudo": relatorio})
	}
}

// Envolver atende as duas rotas internas antes do roteador do serviço, que
// costuma exigir JWT em tudo que não reconhece como público.
func Envolver(s core.Servico, prefixo string, v *internalauth.Verificador, proximo http.Handler) http.Handler {
	interno := http.NewServeMux()
	interno.HandleFunc("GET "+prefixo+CaminhoExportacao, HandlerExportacao(s, v))
	interno.HandleFunc("POST "+prefixo+CaminhoExpurgo, HandlerExpurgo(s, v))
	instrumentado := observabilidade.MiddlewareHTTP(nomeDoServico(prefixo))(observabilidade.Rotas(interno))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == prefixo+CaminhoExportacao,
			r.Method == http.MethodPost && r.URL.Path == prefixo+CaminhoExpurgo:
			instrumentado.ServeHTTP(w, r)
		default:
			proximo.ServeHTTP(w, r)
		}
	})
}

func nomeDoServico(prefixo string) string {
	if nome, _, _ := strings.Cut(strings.Trim(prefixo, "/"), "/"); nome != "" {
		return nome
	}
	return "servico"
}
