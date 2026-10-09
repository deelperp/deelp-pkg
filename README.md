# deelp-pkg

Pacotes compartilhados entre os microserviços da plataforma Deelp.

**Módulo:** `github.com/deelperp/deelp-pkg`
**Repositório:** `github.com/deelperp/deelp-pkg` (privado)

## Estrutura

```
deelp-pkg/
├── go.mod                         module github.com/deelperp/deelp-pkg
├── assinatura/       import "github.com/deelperp/deelp-pkg/assinatura"
├── auth/             import "github.com/deelperp/deelp-pkg/auth"
├── authz/            import "github.com/deelperp/deelp-pkg/authz"
├── cache/            import "github.com/deelperp/deelp-pkg/cache"
├── consumo/          import "github.com/deelperp/deelp-pkg/consumo"
├── dfe/              import "github.com/deelperp/deelp-pkg/dfe"
├── internalauth/     import "github.com/deelperp/deelp-pkg/internalauth"
├── mensageria/       import "github.com/deelperp/deelp-pkg/mensageria"
├── mongodb/          import "github.com/deelperp/deelp-pkg/mongodb"
├── observabilidade/  import "github.com/deelperp/deelp-pkg/observabilidade"
├── postgres/         import "github.com/deelperp/deelp-pkg/postgres"
├── resposta/         import "github.com/deelperp/deelp-pkg/resposta"
├── s3/               import "github.com/deelperp/deelp-pkg/s3"
├── seguranca/        import "github.com/deelperp/deelp-pkg/seguranca"
├── telefone/         import "github.com/deelperp/deelp-pkg/telefone"
├── tenantdados/      import "github.com/deelperp/deelp-pkg/tenantdados"
└── xmldsig/          import "github.com/deelperp/deelp-pkg/xmldsig"
```

Módulo único, **uma versão para todos os subpacotes**. Quando promovem-se mudanças,
a tag é única (`v0.2.0`) e cobre todos os subdiretórios. Isso reduz a fricção de
versionamento individual e funciona bem para o time pequeno do Deelp.

## Pacotes

| Pacote | Resumo |
|---|---|
| `assinatura` | Verificação de assinatura comercial e bloqueio por plano |
| `auth` | JWT middleware (Autenticacao + TenantGuard) + ValidarToken + context helpers |
| `authz` | Consulta remota de permissões; cache com teto e chave sem bearer cru, suporte sem cache |
| `cache` | Cliente Redis padronizado (go-redis/v9) |
| `consumo` | Verificação e registro de consumo por empresa |
| `dfe` | Chave S3 canônica de NF-e / MDF-e / NFS-e (`envio` / `proc` / `eventos`) |
| `internalauth` | Autenticação de chamadas internas entre serviços |
| `mensageria` | Conexão RabbitMQ com reconexão + `Publisher` com confirmação, mandatory e erros tipados |
| `mongodb` | Cliente Mongo + pool tuning + URI ou Host/Port |
| `observabilidade` | OpenTelemetry (traces + metrics + W3C propagator) |
| `postgres` | Cliente Postgres + pool tuning + SSLMode |
| `resposta` | Envelope JSON canônico `{sucesso, mensagem, conteudo}` e writers HTTP (`EscreverErro`, `EscreverResultado`, `EmpresaIdDoToken`) |
| `s3` | Cliente AWS S3; `Open` não altera o bucket, `ConfigurarCORS` é explícito (`NewCliente` deprecated) |
| `seguranca` | Rate limiter (Redis-backed), IPBlocker, SecurityAudit, IPDoRequest |
| `telefone` | Normalização e validação de telefone |
| `tenantdados` | Fachada de exportação/expurgo; núcleo sem drivers em `tenantdados/core`, adapters em `postgres`, `mongo`, `transporte` |
| `xmldsig` | Assinatura digital de XML fiscal |

Handlers HTTP reusam `resposta` na fronteira:

```go
empresaId, ok := resposta.EmpresaIdDoToken(w, r)
if !ok {
    return
}
resposta.EscreverErro(w, http.StatusBadRequest, "Corpo inválido")
resposta.EscreverSucesso(w, conteudo)
resposta.EscreverResultado(w, res.Sucesso, res.Mensagem, res.Conteudo)
resposta.EscreverSaida(w, res.Sucesso, res)
resposta.EscreverCriado(w, res.Sucesso, res)
auth.Config{Responder: resposta.EscreverErro}
```

Finalidade, consumidores, dependências permitidas e estabilidade de cada
pacote: [docs/pacotes.md](docs/pacotes.md). Decisões de arquitetura:
[docs/adr](docs/adr) (limites, nomenclatura, erros, ciclo de vida, mensageria).

## Validação automatizada

O workflow `.github/workflows/quality.yml` valida pull requests e pushes em
`main`, usando a versão de Go declarada no `go.mod` e `GOWORK=off`. Executa
verificação dos módulos, `gofmt`, `go vet ./...` e `go test -race -count=1 ./...`.
As actions são fixadas por SHA.
Os testes HTTP usam servidores locais; não exigem serviços externos.
A matriz de compatibilidade valida os 16 consumidores em workspaces
temporários contendo apenas o candidato de `pkg` e cada serviço.
A matriz também roda `cmd/deelp-arquitetura`: domínio não importa
aplicação/adapters/infra e aplicação não importa adapters/infra. As violações
legadas ficam em `.github/arquitetura/<servico>.txt`; violação nova reprova,
e a linha do baseline só pode ser removida. O job `Integração / RabbitMQ`
testa o `Publisher` contra RabbitMQ real.
Veja [execução local e configuração do CI](docs/compatibility.md).

`authz.HTTPChecker` mantém o cache de permissões das sessões normais, mas
revalida sessões de suporte em toda chamada. O chamador deve propagar o
contexto autenticado por `auth.Autenticacao` ou `auth.ComClaims` após validar
o JWT. Encerrar suporte ou revogar escrita precisa valer na próxima consulta,
inclusive quando já existe uma entrada de cache para o mesmo token.

## Transporte OpenTelemetry

`observabilidade.Iniciar` mantém os providers globais de traces e métricas.
O endpoint determina o transporte quando `Protocolo` é vazio ou `ProtocoloAuto`:

| Endpoint | Protocolo automático | Transporte |
|---|---|---|
| `collector:4317` | gRPC | Sem TLS, compatível com os serviços atuais |
| `http://collector:4318` | HTTP | Sem TLS |
| `https://collector:4318` | HTTP | TLS com validação do certificado |
| `collector:4317` + `TLSConfig` | gRPC | TLS com a configuração fornecida |

`ProtocoloGRPC` explícito também aceita `https://collector:4317` para usar TLS.
Para uma CA privada ou mTLS, forneça `TLSConfig: &tls.Config{RootCAs: roots}`
e, se necessário, `Certificates`. A configuração é clonada ao iniciar.
HTTP com prefixo, como `https://collector/otel`, exporta para
`/otel/v1/traces` e `/otel/v1/metrics`. URLs gRPC não aceitam prefixo.
Credenciais na URL, query, fragmento, protocolo desconhecido e a combinação
`http://` com `TLSConfig` e `TLSConfig.InsecureSkipVerify` são rejeitados antes
da criação dos exporters.

Os exporters HTTP v1.27 podem herdar transporte inseguro de variáveis
`OTEL_EXPORTER_OTLP_*`, mesmo quando TLS é configurado pela aplicação.
Quando isso conflita com TLS solicitado, a exportação falha antes de abrir
a conexão; remova o endpoint `http://` ou a opção `INSECURE` conflitante do
ambiente. A proteção também bloqueia redirecionamento para HTTP. Nenhum
certificado é aceito automaticamente e não há fallback para plaintext.
O startup dos exporters é assíncrono: inicialização bem-sucedida não prova
conectividade com o collector; observe os erros de exportação e de shutdown.

## Desenvolvimento local

Os serviços Deelp estão em repositórios separados. Para iterar localmente
sobre `deelp-pkg` enquanto desenvolve em um serviço, use Go workspace:

```bash
# uma única vez, na pasta-pai que contém todos os clones:
cat > go.work <<'EOF'
go 1.27.2
use (
    ./deelp-pkg
    ./ordem-service
    ./estoque-service
    # ... outros services
)
EOF
```

Com o workspace ativo, mudanças em `deelp-pkg/auth/middleware.go` refletem
imediatamente em qualquer serviço que importe — sem `go get`, sem tagear.

Para validar que o build "limpo" funcionaria em CI:

```bash
GOWORK=off go build ./...
```

## CI/CD nos serviços consumidores

Cada repositório de serviço precisa:

1. **`GOPRIVATE=github.com/deelperp/*`** no ambiente do runner.
2. **Token GitHub** com permissão de leitura em `deelp-pkg` (PAT ou GitHub App).
3. Configuração de Git para autenticar fetch:
   ```bash
   git config --global url."https://${GH_PAT}@github.com/".insteadOf "https://github.com/"
   ```

Exemplo no GitHub Actions:

```yaml
- name: Setup Go
  uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6.5.0
  with:
    go-version: '1.27.2'
    cache: true
- name: Configure private modules
  env:
    GH_PAT: ${{ secrets.GH_PAT_DEELP_PKG }}
  run: |
    git config --global url."https://x-access-token:${GH_PAT}@github.com/".insteadOf "https://github.com/"
    echo "GOPRIVATE=github.com/deelperp/*" >> $GITHUB_ENV
```

No Dockerfile, o token entra como BuildKit secret: existe só durante o `RUN`
que baixa os módulos e não fica em camada, histórico nem cache de build.

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.27.2 AS builder
WORKDIR /app
ENV GOPRIVATE=github.com/deelperp/*
COPY go.mod go.sum ./
RUN --mount=type=secret,id=gh_pat,required=true \
    export GH_PAT="$(cat /run/secrets/gh_pat)" \
    && GIT_CONFIG_COUNT=1 \
       GIT_CONFIG_KEY_0="url.https://x-access-token:${GH_PAT}@github.com/.insteadOf" \
       GIT_CONFIG_VALUE_0="https://github.com/" \
       go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o api
```

Build com:

```bash
GH_PAT=... docker build --secret id=gh_pat,env=GH_PAT --file Dockerfile.prod -t imagem .
```

Não usar `--build-arg GH_PAT`: o valor fica no histórico da camada do builder e
no cache. Os serviços também têm `.github/workflows/quality.yml` (PR e push em
`main`: módulos, gofmt, vet, `-race`).

## Promover nova versão

Atalho (compila, testa, commit+push de `main` se houver mudanças, cria tag):

```bash
./scripts/release-deelp-pkg.sh minor --mensagem "feat(dfe): chave S3 canônica"
# ou só tagear o que já está em main:
./scripts/tag-deelp-pkg.sh patch --com-testes
```

Passo a passo manual:

1. Faça PR em `deelp-pkg`, mergeia em `main`.
2. Crie tag:
   ```bash
   git tag v0.2.0
   git push origin v0.2.0
   ```
3. Em cada serviço que vai bumpar:
   ```bash
   go get github.com/deelperp/deelp-pkg@v0.2.0
   go mod tidy
   git commit -am "chore: bump deelp-pkg para v0.2.0"
   ```

Ou propague nos 16 serviços Go de uma vez:

```bash
./scripts/upgrade-versao-pkg.sh v0.4.0
./scripts/commit-push-upgrade-pkg.sh v0.4.0
```

Use versionamento semântico: `v1.x.x` é API estável; quebra de API exige
mudar o module path para `github.com/deelperp/deelp-pkg/v2`.

## Regras de contrato (importantes)

1. **`deelp-pkg/*` NUNCA importa de `deelp/<service>`.** Se precisa de um tipo
   de serviço, redesenhe para receber via interface/parâmetro.
2. **`deelp-pkg/*` NUNCA lê `os.Getenv` por dentro.** Configuração entra por
   struct `Config`. Quem instancia decide de onde vêm os valores.
3. **Testes não dependem de rede.** Use mocks/fakes; testes que exigem
   Postgres/Mongo/Redis ficam em integração separada.
