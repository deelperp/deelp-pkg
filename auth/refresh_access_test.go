package auth

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRefreshTokenNaoAutorizaRotaDeUsuario(t *testing.T) {
	token := gerarToken(t, jwt.MapClaims{"usuarioId": "usuario", "typ": "refresh", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := ValidarToken(token, secret); !errors.Is(err, ErrTokenInvalido) {
		t.Fatalf("refresh aceito como acesso: %v", err)
	}
	requisicao := httptest.NewRequest(http.MethodGet, "/usuarios/me/sessoes", nil)
	requisicao.Header.Set("Authorization", "Bearer "+token)
	resposta := httptest.NewRecorder()
	Autenticacao(Config{SecretKey: secret})(handlerOK()).ServeHTTP(resposta, requisicao)
	if resposta.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, recebido %d", resposta.Code)
	}
}
