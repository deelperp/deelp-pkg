package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Erro é o payload JSON padrão retornado pelos middlewares quando a request
// é rejeitada. Os 3 serviços hoje usam o mesmo schema {sucesso, mensagem},
// então virou o default. Se algum serviço precisar de um schema diferente,
// pode fornecer Config.Responder.
type Erro struct {
	Sucesso  bool   `json:"sucesso"`
	Mensagem string `json:"mensagem"`
}

// Responder é a função usada para escrever respostas de erro. Quando nil,
// o middleware usa um responder padrão que serializa Erro como JSON.
type Responder func(w http.ResponseWriter, status int, mensagem string)

// Config configura o middleware de autenticação.
//
// SecretKey é obrigatório — deve corresponder ao secret usado pelo
// autenticacao-service para assinar tokens. Não há fallback.
//
// Responder e Logger são opcionais.
type Config struct {
	SecretKey string
	Responder Responder
	Logger    *slog.Logger
}

func responderPadrao(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Erro{Sucesso: false, Mensagem: msg})
}

func (c Config) responder() Responder {
	if c.Responder != nil {
		return c.Responder
	}
	return responderPadrao
}

func (c Config) log(msg string, args ...any) {
	if c.Logger == nil {
		return
	}
	c.Logger.Warn(msg, args...)
}

// ValidarToken parseia e valida um JWT (Bearer ou não), retornando as
// Claims extraídas. Útil para serviços com middleware HTTP próprio que
// querem reusar a lógica de parsing/validação (algoritmo HMAC, claims
// padronizadas) sem adotar o Autenticacao/TenantGuard inteiros.
//
// Devolve erro distinguível:
//   - ErrTokenAusente: tokenString vazio
//   - ErrSecretAusente: secret vazio
//   - ErrTokenInvalido: parse falhou ou token inválido
func ValidarToken(tokenString, secret string) (Claims, error) {
	tokenString = strings.TrimSpace(strings.TrimPrefix(tokenString, "Bearer "))
	if tokenString == "" {
		return Claims{}, ErrTokenAusente
	}
	if secret == "" {
		return Claims{}, ErrSecretAusente
	}

	parsed, err := jwt.ParseWithClaims(tokenString, &jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("algoritmo de assinatura inválido")
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		return Claims{}, ErrTokenInvalido
	}
	c := extrairClaims(parsed)
	if c.UsuarioId == "" {
		return c, ErrTokenInvalido
	}
	return c, nil
}

var (
	ErrTokenAusente  = errors.New("auth: token ausente")
	ErrSecretAusente = errors.New("auth: secret ausente")
	ErrTokenInvalido = errors.New("auth: token inválido ou expirado")
)

func extrairClaims(token *jwt.Token) Claims {
	mc, ok := token.Claims.(*jwt.MapClaims)
	if !ok || mc == nil {
		return Claims{}
	}
	get := func(k string) string {
		v, _ := (*mc)[k].(string)
		return v
	}
	return Claims{
		UsuarioId:      get("usuarioId"),
		Email:          get("email"),
		EmpresaId:      get("empresaId"),
		ColaboracaoId:  get("colaboracaoId"),
		DepartamentoId: get("departamentoId"),
		CargoId:        get("cargoId"),
	}
}

// Autenticacao valida o JWT do header Authorization (Bearer ...), injeta as
// claims no context e segue para o próximo handler. Devolve 401 para token
// ausente, inválido ou expirado, e 500 quando o SecretKey não está configurado.
//
// Importante: exige algoritmo HMAC. Isso bloqueia o ataque "alg: none" e
// também garante que tokens assinados com chaves RSA/EC sejam rejeitados.
func Autenticacao(cfg Config) func(http.Handler) http.Handler {
	resp := cfg.responder()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
			if raw == "" {
				resp(w, http.StatusUnauthorized, "Você precisa estar logado para realizar esta ação")
				return
			}
			if cfg.SecretKey == "" {
				cfg.log("auth.Autenticacao: SecretKey vazio")
				resp(w, http.StatusInternalServerError, "Erro de configuração: SECRET_KEY não configurada")
				return
			}

			parsed, err := jwt.ParseWithClaims(raw, &jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, errors.New("algoritmo de assinatura inválido")
				}
				return []byte(cfg.SecretKey), nil
			})
			if err != nil || !parsed.Valid {
				resp(w, http.StatusUnauthorized, "Sessão expirada. Por favor, faça login novamente")
				return
			}

			claims := extrairClaims(parsed)
			if claims.UsuarioId == "" {
				resp(w, http.StatusUnauthorized, "Token sem identificação do usuário")
				return
			}

			next.ServeHTTP(w, r.WithContext(ComClaims(r.Context(), claims)))
		})
	}
}

// TenantGuard valida que o {empresaId} do path corresponde ao empresaId do
// claim do JWT. Quando o path não contém empresaId, o middleware é no-op
// — rotas que recebem empresaId no body devem fazer cross-check no próprio
// handler.
//
// IMPORTANTE: r.PathValue só é populado APÓS o ServeMux dispatchar. Quando
// este middleware envolve o ServeMux como um todo, r.PathValue retorna
// vazio. Por isso, além de tentar r.PathValue, o middleware faz uma
// varredura defensiva no path bruto procurando qualquer segmento UUID que
// venha imediatamente após um segmento conhecido como prefixo de recurso
// escopado por empresa. Se encontrar e divergir do claim, retorna 403.
//
// Deve ser registrado APÓS Autenticacao na cadeia.
func TenantGuard(cfg Config) func(http.Handler) http.Handler {
	resp := cfg.responder()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pathEmp := r.PathValue("empresaId")
			if pathEmp == "" {
				pathEmp = extrairEmpresaIdDoPath(r.URL.Path)
			}
			if pathEmp == "" {
				next.ServeHTTP(w, r)
				return
			}
			claims, ok := ClaimsDoContexto(r.Context())
			if !ok {
				resp(w, http.StatusUnauthorized, "Claims ausentes no contexto")
				return
			}
			if claims.EmpresaId == "" {
				resp(w, http.StatusForbidden, "Token sem vínculo de empresa. Selecione uma colaboração novamente.")
				return
			}
			if !strings.EqualFold(claims.EmpresaId, pathEmp) {
				resp(w, http.StatusForbidden, "Acesso negado: empresa solicitada não corresponde à sessão atual")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extrairEmpresaIdDoPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		switch parts[i] {
		case "empresas", "empresa":
			if pareceUUID(parts[i+1]) {
				return parts[i+1]
			}
		}
	}
	return ""
}

func pareceUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}
