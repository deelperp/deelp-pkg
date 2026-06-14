# deelp-pkg

Pacotes compartilhados entre os microserviços da plataforma Deelp.

**Módulo:** `github.com/deelperp/deelp-pkg`
**Repositório:** `github.com/deelperp/deelp-pkg` (privado)

## Estrutura

```
deelp-pkg/
├── go.mod                         module github.com/deelperp/deelp-pkg
├── auth/             import "github.com/deelperp/deelp-pkg/auth"
├── cache/            import "github.com/deelperp/deelp-pkg/cache"
├── mensageria/       import "github.com/deelperp/deelp-pkg/mensageria"
├── mongodb/          import "github.com/deelperp/deelp-pkg/mongodb"
├── observabilidade/  import "github.com/deelperp/deelp-pkg/observabilidade"
├── postgres/         import "github.com/deelperp/deelp-pkg/postgres"
├── s3/               import "github.com/deelperp/deelp-pkg/s3"
└── seguranca/        import "github.com/deelperp/deelp-pkg/seguranca"
```

Módulo único, **uma versão para todos os subpacotes**. Quando promovem-se mudanças,
a tag é única (`v0.2.0`) e cobre todos os subdiretórios. Isso reduz a fricção de
versionamento individual e funciona bem para o time pequeno do Deelp.

## Pacotes

| Pacote | Resumo |
|---|---|
| `auth` | JWT middleware (Autenticacao + TenantGuard) + ValidarToken + context helpers |
| `cache` | Cliente Redis padronizado (go-redis/v9) |
| `mensageria` | Conexão RabbitMQ + helpers de exchange/queue |
| `mongodb` | Cliente Mongo + pool tuning + URI ou Host/Port |
| `observabilidade` | OpenTelemetry (traces + metrics + W3C propagator) |
| `postgres` | Cliente Postgres + pool tuning + SSLMode |
| `s3` | Cliente AWS S3 (upload/download/presigned/CORS) |
| `seguranca` | Rate limiter (Redis-backed), IPBlocker, SecurityAudit, IPDoRequest |

## Desenvolvimento local

Os serviços Deelp estão em repositórios separados. Para iterar localmente
sobre `deelp-pkg` enquanto desenvolve em um serviço, use Go workspace:

```bash
# uma única vez, na pasta-pai que contém todos os clones:
cat > go.work <<'EOF'
go 1.26.3
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
  uses: actions/setup-go@v5
  with:
    go-version: '1.26.3'
    cache: true
- name: Configure private modules
  env:
    GH_PAT: ${{ secrets.GH_PAT_DEELP_PKG }}
  run: |
    git config --global url."https://x-access-token:${GH_PAT}@github.com/".insteadOf "https://github.com/"
    echo "GOPRIVATE=github.com/deelperp/*" >> $GITHUB_ENV
```

No Dockerfile:

```dockerfile
FROM golang:1.26.3 AS builder
ARG GH_PAT
WORKDIR /app
RUN git config --global url."https://x-access-token:${GH_PAT}@github.com/".insteadOf "https://github.com/"
ENV GOPRIVATE=github.com/deelperp/*
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o api
```

Build com:

```bash
docker build --build-arg GH_PAT=$GH_PAT --file Dockerfile.prod -t imagem .
```

## Promover nova versão

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

Use versionamento semântico: `v1.x.x` é API estável; quebra de API exige
mudar o module path para `github.com/deelperp/deelp-pkg/v2`.

## Regras de contrato (importantes)

1. **`deelp-pkg/*` NUNCA importa de `deelp/<service>`.** Se precisa de um tipo
   de serviço, redesenhe para receber via interface/parâmetro.
2. **`deelp-pkg/*` NUNCA lê `os.Getenv` por dentro.** Configuração entra por
   struct `Config`. Quem instancia decide de onde vêm os valores.
3. **Testes não dependem de rede.** Use mocks/fakes; testes que exigem
   Postgres/Mongo/Redis ficam em integração separada.
