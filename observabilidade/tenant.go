package observabilidade

import (
	"context"
	"net/http"

	"github.com/deelperp/deelp-pkg/auth"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	AtributoEmpresa = "deelp.empresa_id"
	AtributoUsuario = "deelp.usuario_id"
)

// AtributosTenant só vai em span, nunca em label de métrica (cardinalidade).
func AtributosTenant(ctx context.Context) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	if id, ok := auth.EmpresaIdDoContexto(ctx); ok {
		attrs = append(attrs, attribute.String(AtributoEmpresa, id.String()))
	}
	if id, ok := auth.UsuarioIdDoContexto(ctx); ok {
		attrs = append(attrs, attribute.String(AtributoUsuario, id.String()))
	}
	return attrs
}

type processadorTenant struct{}

func (processadorTenant) OnStart(ctx context.Context, s sdktrace.ReadWriteSpan) {
	redigirAtributosDeURL(s)
	if attrs := AtributosTenant(ctx); len(attrs) > 0 {
		s.SetAttributes(attrs...)
	}
}
func (processadorTenant) OnEnd(sdktrace.ReadOnlySpan)      {}
func (processadorTenant) Shutdown(context.Context) error   { return nil }
func (processadorTenant) ForceFlush(context.Context) error { return nil }

// MarcarTenant vai depois de auth.Autenticacao/TenantGuard: o span do servidor nasce antes
// da autenticação, então o processador não enxerga as claims nele.
func MarcarTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attrs := AtributosTenant(r.Context()); len(attrs) > 0 {
			trace.SpanFromContext(r.Context()).SetAttributes(attrs...)
		}
		next.ServeHTTP(w, r)
	})
}
