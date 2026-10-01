package tenantdados

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/deelperp/deelp-pkg/internalauth"
	"github.com/google/uuid"
)

const (
	CaminhoExportacao = "/interno/tenant/exportacao"
	CaminhoExpurgo    = "/interno/tenant/expurgo"
)

func EscreverZip(w io.Writer, arquivos []Arquivo) error {
	zw := zip.NewWriter(w)
	for _, a := range arquivos {
		f, err := zw.Create(a.Nome)
		if err != nil {
			return err
		}
		if _, err := f.Write(a.Conteudo); err != nil {
			return err
		}
	}
	return zw.Close()
}

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
func (s Servico) HandlerExportacao(v *internalauth.Verificador) http.HandlerFunc {
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
		_ = EscreverZip(w, arquivos)
	}
}

// HandlerExpurgo: POST ?fase=1|2&simular=true|false. Sem simular=false
// explícito, só conta — o padrão nunca apaga.
func (s Servico) HandlerExpurgo(v *internalauth.Verificador) http.HandlerFunc {
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
