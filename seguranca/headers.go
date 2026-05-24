package seguranca

import "net/http"

// HeadersSegurancaConfig configura o middleware de cabecalhos de seguranca HTTP.
//
// Producao=true habilita HSTS. Em dev (HTTP), HSTS quebra o desenvolvimento
// — manter false. O default e' um conjunto restritivo que serve a maioria
// dos servicos REST/JSON sem mexer em assets de frontend.
type HeadersSegurancaConfig struct {
	Producao             bool
	CSP                  string
	ReferrerPolicy       string
	PermissionsPolicy    string
	XFrameOptions        string
	XContentTypeOptions  string
	HSTSMaxAgeSegundos   int
	HSTSIncludeSubDomain bool
}

func (c HeadersSegurancaConfig) comDefaults() HeadersSegurancaConfig {
	if c.CSP == "" {
		c.CSP = "default-src 'self'; frame-ancestors 'none'; base-uri 'self'"
	}
	if c.ReferrerPolicy == "" {
		c.ReferrerPolicy = "strict-origin-when-cross-origin"
	}
	if c.PermissionsPolicy == "" {
		c.PermissionsPolicy = "camera=(), microphone=(), geolocation=()"
	}
	if c.XFrameOptions == "" {
		c.XFrameOptions = "DENY"
	}
	if c.XContentTypeOptions == "" {
		c.XContentTypeOptions = "nosniff"
	}
	if c.HSTSMaxAgeSegundos == 0 {
		c.HSTSMaxAgeSegundos = 31536000
	}
	return c
}

// HeadersSeguranca devolve um middleware que define cabecalhos restritivos.
// Recomendado aplicar logo na entrada do handler, antes de qualquer logica
// que possa retornar erro (assim os headers viajam tambem nas respostas 4xx).
func HeadersSeguranca(cfg HeadersSegurancaConfig) func(http.Handler) http.Handler {
	final := cfg.comDefaults()
	hsts := ""
	if final.Producao {
		hsts = "max-age=" + itoaSimples(final.HSTSMaxAgeSegundos)
		if final.HSTSIncludeSubDomain {
			hsts += "; includeSubDomains"
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", final.CSP)
			h.Set("Referrer-Policy", final.ReferrerPolicy)
			h.Set("Permissions-Policy", final.PermissionsPolicy)
			h.Set("X-Frame-Options", final.XFrameOptions)
			h.Set("X-Content-Type-Options", final.XContentTypeOptions)
			if hsts != "" {
				h.Set("Strict-Transport-Security", hsts)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func itoaSimples(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
