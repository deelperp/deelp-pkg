package observabilidade

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	segmentoUUID     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	segmentoNumerico = regexp.MustCompile(`^[0-9]{14,}$`)
	atributosDeURL   = map[attribute.Key]bool{"url.path": true, "http.target": true, "url.full": true, "http.url": true}
)

func segmentoSensivel(seg string) bool {
	if segmentoUUID.MatchString(seg) || segmentoNumerico.MatchString(seg) {
		return true
	}
	if len(seg) < 24 {
		return false
	}
	temDigito := false
	for _, r := range seg {
		switch {
		case unicode.IsDigit(r):
			temDigito = true
		case unicode.IsLetter(r), r == '-', r == '_', r == '=':
		default:
			return false
		}
	}
	return temDigito
}

func sanitizarPath(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if segmentoSensivel(s) {
			segs[i] = ":id"
		}
	}
	return strings.Join(segs, "/")
}

// sanitizarURL tira credenciais e query, e troca segmentos que parecem id, token ou
// chave de acesso. Token de link público e chave de NF-e vão no path e não podem
// parar no backend de traces.
func sanitizarURL(bruta string) string {
	if !strings.Contains(bruta, "://") {
		caminho, _, _ := strings.Cut(bruta, "?")
		return sanitizarPath(caminho)
	}
	u, err := url.Parse(bruta)
	if err != nil {
		return "[url inválida]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = sanitizarPath(u.Path)
	u.RawPath = ""
	return u.String()
}

func redigirAtributosDeURL(s sdktrace.ReadWriteSpan) {
	if k := s.SpanKind(); k != trace.SpanKindServer && k != trace.SpanKindClient {
		return
	}
	for _, kv := range s.Attributes() {
		if !atributosDeURL[kv.Key] || kv.Value.Type() != attribute.STRING {
			continue
		}
		if limpo := sanitizarURL(kv.Value.AsString()); limpo != kv.Value.AsString() {
			s.SetAttributes(attribute.String(string(kv.Key), limpo))
		}
	}
}
