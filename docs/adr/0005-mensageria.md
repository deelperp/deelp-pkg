# ADR 0005 — Publicação confiável

Situação: aceita (2026-10-03).

## Decisão
- `mensageria.Publisher` só devolve nil após o ack do broker (publisher
  confirms). Mensagem persistente por padrão.
- `mandatory` sempre ligado; mensagem sem fila vira `ErrNaoRoteada`, a menos que
  o produtor marque `PermitirSemRota` (evento de domínio sem consumidor).
- Nack vira `ErrRecusada`; ausência de confirmação vira `ErrConfirmacaoIncerta`
  e não é retentada automaticamente — a mensagem pode ter chegado.
- Canal fechado é reaberto uma vez antes do envio. Conexão é mantida pelo
  `GerenciadorConexao`, que reconecta com backoff.
- Produtor que pode reenviar o mesmo fato usa `MessageId` determinístico para o
  consumidor deduplicar.
- Evento que precisa acompanhar commit de banco exige outbox transacional no
  serviço dono; confirmação não torna banco e broker atômicos.

## Testes
Unitários sem broker; integração com `DEELP_RABBITMQ_URL` (job de CI com
RabbitMQ 3.13). Localmente validado com broker AMQP em Go (garagemq), que não
confirma mensagem persistente — essa parte só o CI cobre.
