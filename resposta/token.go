package resposta

import (
	"net/http"

	"github.com/deelperp/deelp-pkg/auth"
	"github.com/google/uuid"
)

// EmpresaIdDoToken lê o tenant exclusivamente do JWT. Sem claim, responde 403
// e devolve false — o handler encerra. Nunca aceite empresaId de path, query
// ou body: docs/SECURITY-TENANT-ISOLATION.md.
func EmpresaIdDoToken(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := auth.EmpresaIdDoContexto(r.Context())
	if !ok {
		EscreverErro(w, http.StatusForbidden, MsgSemEmpresa)
		return uuid.Nil, false
	}
	return id, true
}

// UsuarioIdDoToken lê o autor exclusivamente do JWT. Sem claim, responde 403.
func UsuarioIdDoToken(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := auth.UsuarioIdDoContexto(r.Context())
	if !ok {
		EscreverErro(w, http.StatusForbidden, MsgSemUsuario)
		return uuid.Nil, false
	}
	return id, true
}
