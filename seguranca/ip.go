package seguranca

import (
	"net/http"
	"strings"
)

// IPDoRequest extrai o IP real do cliente respeitando proxies.
//
// X-Forwarded-For pode conter múltiplos IPs ("real, proxy1, proxy2");
// apenas o primeiro elemento é o IP original do cliente — os demais
// são adicionados por proxies intermediários e podem ser forjados.
// Esta é a única forma correta de tratar o header em ambientes
// multi-camada (CloudFront → ALB → app), e fixa o problema apontado
// no CLAUDE.md ("X-Forwarded-For não sanitizado").
func IPDoRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	if addr := r.RemoteAddr; addr != "" {
		if idx := strings.LastIndex(addr, ":"); idx != -1 {
			return addr[:idx]
		}
		return addr
	}
	return ""
}
