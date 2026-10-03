package observabilidade

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type endpointConfig struct {
	host   string
	path   string
	secure bool
}

func (c Config) resolveEndpoint() (endpointConfig, error) {
	raw := strings.TrimSpace(c.Endpoint)
	hasScheme := strings.Contains(raw, "://")
	if !hasScheme {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return endpointConfig{}, errors.New("observabilidade: endpoint inválido")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return endpointConfig{}, errors.New("observabilidade: endpoint deve usar http ou https")
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return endpointConfig{}, errors.New("observabilidade: endpoint exige host e não aceita credenciais, query ou fragmento")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return endpointConfig{}, errors.New("observabilidade: porta do endpoint inválida")
		}
	}
	if strings.HasSuffix(u.Host, ":") || (strings.Count(u.Host, ":") > 1 && !strings.HasPrefix(u.Host, "[")) {
		return endpointConfig{}, errors.New("observabilidade: host ou porta do endpoint inválido")
	}
	if u.Path != "" && u.Path != "/" && (!hasScheme || c.protocoloEfetivo() == ProtocoloGRPC) {
		return endpointConfig{}, errors.New("observabilidade: prefixo de caminho exige endpoint HTTP com esquema")
	}
	if c.TLSConfig != nil && c.TLSConfig.InsecureSkipVerify {
		return endpointConfig{}, errors.New("observabilidade: TLSConfig com InsecureSkipVerify não é aceito; informe a CA em RootCAs")
	}
	if hasScheme && u.Scheme == "http" && c.TLSConfig != nil {
		return endpointConfig{}, errors.New("observabilidade: TLSConfig é incompatível com endpoint http; use https")
	}
	return endpointConfig{
		host:   u.Host,
		path:   strings.TrimRight(u.Path, "/"),
		secure: u.Scheme == "https" || c.TLSConfig != nil,
	}, nil
}

// Os exporters v1.27 podem herdar Insecure do ambiente mesmo com TLSConfig.
// O proxy é consultado antes de abrir a conexão, impedindo esse downgrade.
func secureProxy(r *http.Request) (*url.URL, error) {
	if r.URL.Scheme != "https" {
		return nil, errors.New("observabilidade: exportação sem TLS bloqueada; remova configuração OTLP insegura do ambiente")
	}
	return http.ProxyFromEnvironment(r)
}
