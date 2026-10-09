package observabilidade

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	duracaoServidor = HistogramaSegundos("deelp_http_request_duration_seconds", "Duração das requisições HTTP atendidas")
	emAndamento     = EmAndamento("deelp_http_requests_in_flight", "Requisições HTTP em andamento")
	duracaoCliente  = HistogramaSegundos("deelp_http_client_duration_seconds", "Duração das chamadas HTTP de saída")
)

const rotaNaoRoteada = "nao_roteada"

type chaveRota struct{}

type referenciaRota struct{ valor atomic.Pointer[string] }

// Rotas embrulha o mux que de fato roteia. Middlewares no meio da cadeia (auth.Autenticacao
// usa r.WithContext) copiam a request, então r.Pattern não chega ao middleware externo;
// o mux informa a rota por uma referência guardada no contexto.
func Rotas(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		if ref, ok := r.Context().Value(chaveRota{}).(*referenciaRota); ok && r.Pattern != "" {
			p := r.Pattern
			ref.valor.CompareAndSwap(nil, &p)
		}
	})
}

func rotaDe(ctx context.Context, r *http.Request) string {
	if ref, ok := ctx.Value(chaveRota{}).(*referenciaRota); ok {
		if p := ref.valor.Load(); p != nil {
			return *p
		}
	}
	if r.Pattern != "" {
		return r.Pattern
	}
	return rotaNaoRoteada
}

func nomeSpanDaRota(metodo, rota string) string {
	if rota == rotaNaoRoteada {
		return metodo + " " + rota
	}
	if i := strings.IndexByte(rota, ' '); i > 0 && !strings.HasPrefix(rota, "/") {
		return rota
	}
	return metodo + " " + rota
}

func ignorarRequisicao(r *http.Request) bool {
	switch r.URL.Path {
	case "/health", "/healthz", "/ready", "/readyz", "/metrics":
		return true
	}
	return strings.Count(r.URL.Path, "/") <= 2 && (strings.HasSuffix(r.URL.Path, "/health") || strings.HasSuffix(r.URL.Path, "/healthz"))
}

func requisicaoPublica(r *http.Request) bool {
	return r.Header.Get("Authorization") == "" && r.Header.Get("X-Internal-Key") == ""
}

func ehUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func metodoConhecido(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return m
	}
	return "_OTHER"
}

func classeStatus(codigo int) string {
	return strconv.Itoa(codigo/100) + "xx"
}

// MiddlewareHTTP é o middleware mais externo: abre o span do servidor, nomeia pela rota
// registrada (r.Pattern, nunca o path com UUID) e registra RED. Precisa envolver o mux,
// porque r.Pattern só é preenchido depois do roteamento.
func MiddlewareHTTP(servico string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		interno := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ignorarRequisicao(r) {
				next.ServeHTTP(w, r)
				return
			}
			ref := &referenciaRota{}
			ctx := context.WithValue(r.Context(), chaveRota{}, ref)
			r = r.WithContext(ctx)
			servicoAttr := metric.WithAttributes(attribute.String("servico", servico))
			emAndamento.Add(ctx, 1, servicoAttr)
			defer emAndamento.Add(ctx, -1, servicoAttr)

			inicio := time.Now()
			codigo := http.StatusInternalServerError
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				if p == http.ErrAbortHandler {
					codigo = 499
				}
				finalizarRequisicao(ctx, r, servico, codigo, time.Since(inicio))
				panic(p)
			}()

			m := httpsnoop.CaptureMetrics(next, w, r)
			codigo = m.Code
			finalizarRequisicao(ctx, r, servico, codigo, m.Duration)
		})
		return otelhttp.NewHandler(interno, servico,
			otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
			otelhttp.WithFilter(func(r *http.Request) bool { return !ignorarRequisicao(r) }),
			otelhttp.WithPublicEndpointFn(requisicaoPublica),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method }),
		)
	}
}

func finalizarRequisicao(ctx context.Context, r *http.Request, servico string, codigo int, duracao time.Duration) {
	rota := rotaDe(ctx, r)
	if rota == rotaNaoRoteada {
		switch {
		case r.Method == http.MethodOptions:
			rota = "preflight_cors"
		case codigo == 401 || codigo == 402 || codigo == 403 || codigo == 429:
			rota = "recusada_antes_da_rota"
		}
	}
	span := trace.SpanFromContext(ctx)
	span.SetName(nomeSpanDaRota(r.Method, rota))
	span.SetAttributes(attribute.String("http.route", rota))
	if codigo >= 500 {
		span.SetStatus(codes.Error, "HTTP "+strconv.Itoa(codigo))
	}
	if ehUpgrade(r) {
		return
	}
	duracaoServidor.Record(ctx, duracao.Seconds(), metric.WithAttributes(
		attribute.String("servico", servico),
		attribute.String("rota", rota),
		attribute.String("metodo", metodoConhecido(r.Method)),
		attribute.String("status_classe", classeStatus(codigo)),
	))
}

type transporteMedido struct {
	base http.RoundTripper
}

func (t transporteMedido) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

func (t transporteMedido) RoundTrip(r *http.Request) (*http.Response, error) {
	inicio := time.Now()
	resp, err := t.base.RoundTrip(r)
	status := "erro"
	if err == nil {
		status = classeStatus(resp.StatusCode)
	}
	duracaoCliente.Record(r.Context(), time.Since(inicio).Seconds(), metric.WithAttributes(
		attribute.String("destino", destinoDe(r.URL)),
		attribute.String("metodo", r.Method),
		attribute.String("status_classe", status),
	))
	return resp, err
}

func destinoDe(u *url.URL) string {
	if u == nil {
		return "desconhecido"
	}
	return u.Host
}

// Transporte propaga traceparent, abre span de cliente e mede a chamada. Destino é o host,
// nunca o path (cardinalidade).
func Transporte(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return transporteMedido{base: otelhttp.NewTransport(base,
		otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return "HTTP " + r.Method + " " + destinoDe(r.URL)
		}),
	)}
}

// TransporteExterno mantém span e métrica, mas não injeta traceparent nem baggage:
// serviço de terceiro (SEFAZ, PSP, consulta de CNPJ, IA) não recebe identificador interno.
func TransporteExterno(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return transporteMedido{base: otelhttp.NewTransport(base,
		otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return "HTTP " + r.Method + " " + destinoDe(r.URL)
		}),
	)}
}

func ClienteHTTP(timeout time.Duration, base http.RoundTripper) *http.Client {
	return &http.Client{Timeout: timeout, Transport: Transporte(base)}
}
