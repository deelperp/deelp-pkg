# Inventário dos pacotes

Responsável por todos: time de plataforma do Deelp (mantenedor do `deelp-pkg`).
Consumidores levantados por import em 2026-10-03. "Estável" significa que mudar
assinatura pública exige fachada de compatibilidade e versão minor; "evoluindo"
aceita adições, nunca remoção sem ciclo de depreciação.

| Pacote | Finalidade | Consumidores | Dependências permitidas | Estabilidade |
|---|---|---|---|---|
| `assinatura` | Bloqueio por contrato comercial (402) e cache da situação do contrato | autenticacao, cliente, estoque, financeiro, mdfe, nfe, nfse, ordem, tarefa | `auth`, `internal/ttlcache`, stdlib | estável |
| `auth` | JWT, `Autenticacao`, `TenantGuard`, claims no contexto, gates de suporte | todos os 16 | jwt, uuid | estável — mudança de gate repete em mdfe e whatsapp |
| `authz` | Permissão remota no autenticacao-service; suporte sem cache | cliente, estoque, financeiro, mdfe, nfe, nfse, ordem, relatorio | `auth`, `internal/ttlcache` | estável |
| `cache` | Cliente Redis padronizado | autenticacao, cliente, estoque, fatura, relatorio, tarefa, wiki | go-redis | estável |
| `consumo` | Registro e limite de consumo por empresa | cliente, mdfe, nfe, nfse, ordem | `auth`, stdlib | evoluindo (negócio compartilhado) |
| `dfe` | Chave S3 canônica de NF-e/MDF-e/NFS-e | mdfe, nfe, nfse | stdlib | estável (negócio compartilhado) |
| `internalauth` | Chave interna entre serviços | 13 serviços | stdlib | estável |
| `mensageria` | Conexão RabbitMQ, reconexão e `Publisher` confirmado | autenticacao, cliente, tarefa | amqp091, otel | evoluindo |
| `mongodb` | Cliente Mongo com pool | 10 serviços | mongo-driver | estável |
| `observabilidade` | OTel traces + métricas via OTLP, TLS explícito | 15 serviços | otel, grpc | estável |
| `postgres` | Pool PostgreSQL | 12 serviços | lib/pq | estável |
| `resposta` | Envelope `{sucesso, mensagem, conteudo}` e writers | todos os 16 | `auth` | estável |
| `s3` | Cliente S3; `Open` sem efeito, `ConfigurarCORS` explícito | 9 serviços | aws-sdk-go-v2 | estável (`NewCliente` deprecated) |
| `seguranca` | Rate limiter, IPBlocker, auditoria, `IPDoRequest` | 15 serviços | go-redis, otel | estável |
| `telefone` | Normalização de telefone | financeiro, mdfe, nfe | stdlib | estável |
| `tenantdados` | Fachada de exportação/expurgo (núcleo em `core`) | 13 serviços | ver subpacotes | estável (fachada) |
| `tenantdados/core` | Contrato e orquestração, sem drivers nem HTTP | via fachada | uuid, stdlib (travado por teste) | evoluindo |
| `tenantdados/postgres`, `/mongo`, `/transporte` | Adapters do núcleo | via fachada | driver respectivo + `core` | evoluindo |
| `xmldsig` | Assinatura XMLDSig de documento fiscal | mdfe, nfe, nfse | stdlib, pkcs12 | estável — revisão fiscal obrigatória |
| `internal/ttlcache` | Cache com teto, TTL e colapso de chamadas | `authz`, `assinatura` | stdlib | interno, sem API pública |
| `cmd/deelp-arquitetura` | Checker de camadas com baseline | CI da matriz | stdlib | ferramenta |

Critério para entrar no `pkg`: consumidor real em pelo menos dois serviços e
semântica estável. Entidade de negócio de um serviço só fica no serviço dono.
