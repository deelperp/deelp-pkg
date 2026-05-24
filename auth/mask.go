package auth

import "strings"

// MascararToken devolve uma representação segura do token para inclusão em
// logs, métricas ou mensagens de erro. Mostra apenas os 6 primeiros caracteres
// e os 4 últimos, separados pelo comprimento total — suficiente para correlação
// sem expor o segredo. Tokens "Bearer ..." são tratados removendo o prefixo
// antes do mascaramento.
//
// Exemplo:
//   MascararToken("Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xxx.yyy")
//   -> "eyJhbG…(123)…yyy"
//
// Sempre prefira esta função a logar o token cru.
func MascararToken(token string) string {
	t := strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if t == "" {
		return "<vazio>"
	}
	n := len(t)
	if n <= 12 {
		return "<curto:" + itoa(n) + ">"
	}
	return t[:6] + "…(" + itoa(n) + ")…" + t[n-4:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negativo := n < 0
	if negativo {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negativo {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
