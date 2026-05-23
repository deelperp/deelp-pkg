package seguranca

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Evento descreve um evento de segurança detectado/registrado.
type Evento struct {
	Timestamp time.Time
	IP        string
	UserAgent string
	Method    string
	Path      string
	Tipo      string // "rate_limit", "ip_blocked", "padrao_suspeito", "login_*"
	UsuarioID string
	Detalhes  map[string]any
}

// AuditConfig configura a auditoria de segurança.
type AuditConfig struct {
	Logger      *slog.Logger
	Padroes     []string // padroes suspeitos (default: ../, <script, etc.)
	OnEvent     func(Evento)
	Responder   Responder
	BloquearReq bool // se true, request com padrao suspeito vira 400; senao só loga
}

// SecurityAudit detecta padrões suspeitos em paths/query e registra eventos.
type SecurityAudit struct {
	cfg       AuditConfig
	padroes   []string
	responder Responder
}

// PadroesDefault é a lista usada quando AuditConfig.Padroes está vazia.
var PadroesDefault = []string{
	"../",
	"<script",
	"union select",
	"'; drop",
	"exec(",
	"eval(",
}

func NewSecurityAudit(cfg AuditConfig) *SecurityAudit {
	sa := &SecurityAudit{cfg: cfg, padroes: cfg.Padroes, responder: cfg.Responder}
	if len(sa.padroes) == 0 {
		sa.padroes = PadroesDefault
	}
	if sa.responder == nil {
		sa.responder = responderPadrao
	}
	return sa
}

// Registrar emite um Evento. Quando cfg.OnEvent é nil, loga via slog.
func (s *SecurityAudit) Registrar(r *http.Request, tipo, usuarioID string, detalhes map[string]any) {
	e := Evento{
		Timestamp: time.Now(),
		IP:        IPDoRequest(r),
		UserAgent: r.UserAgent(),
		Method:    r.Method,
		Path:      r.URL.Path,
		Tipo:      tipo,
		UsuarioID: usuarioID,
		Detalhes:  detalhes,
	}
	if s.cfg.OnEvent != nil {
		s.cfg.OnEvent(e)
	} else if s.cfg.Logger != nil {
		s.cfg.Logger.Warn("seguranca", "tipo", e.Tipo, "ip", e.IP, "path", e.Path, "userId", e.UsuarioID, "detalhes", e.Detalhes)
	}
}

// DetectarSuspeito devolve true se a path/query contém algum dos padrões.
func (s *SecurityAudit) DetectarSuspeito(r *http.Request) (bool, string) {
	alvo := strings.ToLower(r.URL.Path + "?" + r.URL.RawQuery)
	for _, p := range s.padroes {
		if strings.Contains(alvo, p) {
			return true, p
		}
	}
	return false, ""
}

func (s *SecurityAudit) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if susp, pad := s.DetectarSuspeito(r); susp {
			s.Registrar(r, "padrao_suspeito", "", map[string]any{"padrao": pad})
			if s.cfg.BloquearReq {
				s.responder(w, http.StatusBadRequest, "Requisição inválida")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
