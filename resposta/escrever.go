package resposta

import (
	"encoding/json"
	"net/http"
)

// EscreverJSON serializa corpo com Content-Type application/json.
func EscreverJSON(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(corpo)
}

// EscreverErro responde com o envelope de falha. A assinatura casa com
// auth.Config.Responder — o middleware de JWT reusa a mesma rotina.
func EscreverErro(w http.ResponseWriter, status int, mensagem string) {
	EscreverJSON(w, status, Falha(mensagem))
}

// EscreverSucesso responde 200 com o conteúdo informado.
func EscreverSucesso(w http.ResponseWriter, conteudo any) {
	EscreverJSON(w, http.StatusOK, Ok(conteudo))
}

// EscreverResultado infere o status HTTP a partir de sucesso/mensagem e
// escreve o envelope canônico. Use cases devolvem o trio; o handler não
// decide 200 vs 400 à mão.
func EscreverResultado(w http.ResponseWriter, sucesso bool, mensagem string, conteudo any) {
	if sucesso {
		EscreverJSON(w, http.StatusOK, Corpo{Sucesso: true, Mensagem: mensagem, Conteudo: conteudo})
		return
	}
	EscreverErro(w, StatusDoErro(mensagem), mensagem)
}

// EscreverSaida escreve o DTO de resultado já montado pelo use case.
// 200 se sucesso, 400 se falha — o contrato histórico dos handlers Deelp.
func EscreverSaida(w http.ResponseWriter, sucesso bool, saida any) {
	status := http.StatusOK
	if !sucesso {
		status = http.StatusBadRequest
	}
	EscreverJSON(w, status, saida)
}

// EscreverCriado é EscreverSaida com 201 no sucesso (POST que cria recurso).
func EscreverCriado(w http.ResponseWriter, sucesso bool, saida any) {
	status := http.StatusCreated
	if !sucesso {
		status = http.StatusBadRequest
	}
	EscreverJSON(w, status, saida)
}
