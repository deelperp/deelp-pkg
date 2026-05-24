package seguranca

import (
	"context"
	"log/slog"
	"net/http"
)

// EventoSeg representa um evento de segurança com campos padronizados.
// Use este tipo (e a função EmitirEvento) em qualquer ponto que precisar
// logar acontecimentos relevantes para auditoria/SIEM — login, logout,
// rehash de senha, bloqueio de IP, divergência de tenant, etc.
//
// O campo Detalhes é livre, mas evite incluir dados sensíveis (token cru,
// senha, body completo). Use auth.MascararToken se precisar referenciar o
// token.
type EventoSeg struct {
	Evento    string
	UsuarioID string
	EmpresaID string
	IP        string
	UserAgent string
	Path      string
	Metodo    string
	Detalhes  map[string]any
}

// EmitirEvento loga o evento via slog usando a categoria "seguranca". Use o
// ctx do request para que tracing IDs sejam propagados quando o handler
// configurar um slog.Handler com context awareness.
func EmitirEvento(ctx context.Context, e EventoSeg) {
	attrs := []slog.Attr{
		slog.String("evento", e.Evento),
	}
	if e.UsuarioID != "" {
		attrs = append(attrs, slog.String("usuario_id", e.UsuarioID))
	}
	if e.EmpresaID != "" {
		attrs = append(attrs, slog.String("empresa_id", e.EmpresaID))
	}
	if e.IP != "" {
		attrs = append(attrs, slog.String("ip", e.IP))
	}
	if e.UserAgent != "" {
		attrs = append(attrs, slog.String("user_agent", e.UserAgent))
	}
	if e.Path != "" {
		attrs = append(attrs, slog.String("path", e.Path))
	}
	if e.Metodo != "" {
		attrs = append(attrs, slog.String("metodo", e.Metodo))
	}
	for k, v := range e.Detalhes {
		attrs = append(attrs, slog.Any(k, v))
	}
	slog.LogAttrs(ctx, slog.LevelInfo, "seguranca", attrs...)
	servico := ""
	if v, ok := e.Detalhes["servico"].(string); ok {
		servico = v
	}
	MetricaEvento(ctx, e.Evento, servico)
}

// EmitirEventoRequest é um atalho que preenche IP, UserAgent, Path e Método
// a partir do request HTTP.
func EmitirEventoRequest(r *http.Request, e EventoSeg) {
	if r != nil {
		if e.IP == "" {
			e.IP = IPDoRequest(r)
		}
		if e.UserAgent == "" {
			e.UserAgent = r.UserAgent()
		}
		if e.Path == "" {
			e.Path = r.URL.Path
		}
		if e.Metodo == "" {
			e.Metodo = r.Method
		}
		EmitirEvento(r.Context(), e)
		return
	}
	EmitirEvento(context.Background(), e)
}
