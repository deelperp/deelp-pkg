package resposta

import (
	"net/http"
	"strings"
)

// StatusDoErro traduz a mensagem de negócio no código HTTP correspondente.
// "não encontrado" e "conflito/já existe" são os únicos casos com semântica
// própria; o restante cai em 400 — falha de validação, não incidente.
func StatusDoErro(mensagem string) int {
	msg := strings.ToLower(mensagem)
	switch {
	case strings.Contains(msg, "não encontrad") || strings.Contains(msg, "nao encontrad"):
		return http.StatusNotFound
	case strings.Contains(msg, "não pode ser editad") ||
		strings.Contains(msg, "nao pode ser editad") ||
		strings.Contains(msg, "já exist") ||
		strings.Contains(msg, "ja exist") ||
		strings.Contains(msg, "conflito"):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
