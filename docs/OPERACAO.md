# Operação: saúde, métricas e logs

Endpoints e cuidados para quem opera o servidor. Contexto do que foi feito e do que falta: [PROGRESSO.md](PROGRESSO.md).

## Saúde

| Rota | Autenticação | Para que serve |
|---|---|---|
| `GET /server/ok` | nenhuma | **Liveness.** Responde 200 se o processo está de pé. Não consulta nada. |
| `GET /health` | nenhuma | **Readiness.** Confere se os bancos respondem e conta os clientes do WhatsApp. |

Exemplo de `GET /health`:

```json
{
  "status": "ok",
  "checks": {
    "database":     { "status": "up", "latencyMs": 2 },
    "authDatabase": { "status": "up", "latencyMs": 1 }
  },
  "clients": { "total": 3, "connected": 2, "loggedIn": 2 }
}
```

- `database` é o banco de usuários e instâncias; `authDatabase` aparece só com `POSTGRES_AUTH_DB` (no SQLite não há esse banco).
- Se um banco não responde em 3 segundos, o status vira `unavailable` e a resposta é **503**. O corpo diz só `ping failed`; o motivo detalhado fica no log do servidor, porque o endpoint é público.
- `clients` conta o que está em memória. Instância desconectada **não** deixa o servidor `unavailable`: ela pode estar desconectada de propósito. Quem quer alertar sobre isso usa as métricas.
- O gate de licença deixa `/health` passar mesmo sem licença ativa. Assim um orquestrador não reinicia um processo saudável só porque falta ativar a licença.

Em Docker Compose / Swarm:

```yaml
healthcheck:
  test: ["CMD", "wget", "-q", "--spider", "http://localhost:4000/health"]
  interval: 30s
  timeout: 5s
  retries: 3
```

## Métricas

`GET /metrics`, no formato de texto do Prometheus. Exige a chave global no header `apikey` (as métricas mostram a carga do servidor e não devem ficar públicas). Sem licença ativa, o gate de licença responde 503 (`LICENSE_REQUIRED`) antes de conferir a chave; com a licença ativa, chave ausente ou errada dá 401.

```bash
curl -H "apikey: $GLOBAL_API_KEY" http://localhost:4000/metrics
```

| Métrica | Tipo | O que mostra |
|---|---|---|
| `evogo_clients` | gauge | Clientes do WhatsApp em memória |
| `evogo_clients_connected` | gauge | Os que estão com o websocket conectado |
| `evogo_clients_logged_in` | gauge | Os que estão com a conta logada |
| `evogo_instance_reconnects_total` | counter | Reconexões iniciadas (automáticas ou pela API) |
| `evogo_instance_disconnects_total` | counter | Quedas inesperadas de conexão |
| `evogo_webhook_delivered_total` | counter | Webhooks entregues (2xx) |
| `evogo_webhook_attempt_failures_total` | counter | Tentativas de webhook que falharam (cada webhook tenta até 5 vezes) |
| `evogo_webhook_given_up_total` | counter | Webhooks descartados depois da última tentativa |
| `evogo_send_queue_waiting` | gauge | Mensagens esperando a vez na fila de envio agora |
| `evogo_send_queue_delayed_total` | counter | Mensagens que esperaram a vez |
| `evogo_send_queue_wait_milliseconds_total` | counter | Tempo total esperado na fila |
| `evogo_send_queue_rejected_full_total` | counter | Recusadas porque a fila da instância estava cheia |
| `evogo_send_queue_rejected_timeout_total` | counter | Recusadas porque a vez passaria do tempo máximo |
| `evogo_db_open_connections{pool}` | gauge | Conexões abertas de cada pool (`database`, `authDatabase`) |
| `evogo_db_in_use_connections{pool}` | gauge | Conexões em uso |
| `evogo_db_idle_connections{pool}` | gauge | Conexões ociosas |
| `evogo_db_max_open_connections{pool}` | gauge | Limite do pool |
| `evogo_db_wait_count_total{pool}` | counter | Vezes que se esperou por uma conexão |
| `evogo_db_wait_seconds_total{pool}` | counter | Tempo esperando por uma conexão |

O que vigiar (foram os defeitos que só apareceram tarde no upstream):

- **Vazamento de pool:** `evogo_db_open_connections` crescendo a cada reconexão e nunca voltando; ou `evogo_db_open_connections` encostado em `evogo_db_max_open_connections` com `evogo_db_wait_count_total` subindo.
- **Loop de reconexão:** `rate(evogo_instance_disconnects_total[5m])` e `rate(evogo_instance_reconnects_total[5m])` altos e constantes.
- **Instâncias fora:** `evogo_clients - evogo_clients_connected` diferente de zero por muito tempo.
- **Receptor de webhook com problema:** `rate(evogo_webhook_attempt_failures_total[5m])` subindo; `evogo_webhook_given_up_total` subindo significa eventos perdidos.

Os contadores zeram quando o processo reinicia (comportamento normal de contador do Prometheus, que `rate()` já trata). O pool do `sqlstore` do whatsmeow com PostgreSQL é o mesmo `authDatabase`. Com SQLite o whatsmeow abre o próprio pool e ele não aparece aqui.

## Fila de envio

Disparar muitas mensagens seguidas por uma conta é o caminho mais curto para o erro 463 e para a conta restrita (#236, #50). O servidor não tinha freio: quem envia em lote precisava se controlar do lado de fora. A fila de envio espaça as mensagens **de cada instância**, com intervalo aleatório entre um mínimo e um máximo (intervalo fixo é fácil de reconhecer como automação).

**Vem desligada.** Ligar é decisão de quem opera o servidor:

| Variável | Padrão | O que faz |
|---|---|---|
| `SEND_QUEUE_MIN_INTERVAL_MS` | `0` | Menor intervalo entre duas mensagens da mesma instância. **Zero desliga a fila.** |
| `SEND_QUEUE_MAX_INTERVAL_MS` | `0` | Maior intervalo. O de cada mensagem é sorteado entre o mínimo e este valor. Abaixo do mínimo, vale o mínimo. |
| `SEND_QUEUE_MAX_PENDING` | `100` | Quantas mensagens podem esperar a vez por instância. `0` = sem limite. |
| `SEND_QUEUE_MAX_WAIT_SECONDS` | `50` | Quanto uma mensagem espera, no máximo, pela vez. `0` = sem limite. |

Como funciona:

- Cada mensagem reserva o próximo horário livre da sua instância e espera por ele. A primeira sai na hora; quem chega depois sai depois, na ordem. Instâncias diferentes não se atrapalham.
- A requisição HTTP **fica aberta** até a mensagem sair, e a resposta é a mesma de sempre (com o ID da mensagem). Nenhum formato de resposta muda.
- Se a fila da instância já está cheia (`SEND_QUEUE_MAX_PENDING`) ou a vez só chegaria depois de `SEND_QUEUE_MAX_WAIT_SECONDS`, a API responde **429** na hora, com o cabeçalho `Retry-After` (segundos) e `retryAfter` no corpo. Sem isso, o cliente ficaria com a requisição pendurada até o próprio tempo limite e tentaria de novo, piorando a fila. Quem envia em lote deve tratar o 429 esperando o `Retry-After`.
- **Deixe `SEND_QUEUE_MAX_WAIT_SECONDS` abaixo do tempo limite de quem chama a API.** Por exemplo, o Whatsflow limita cada envio a 60 segundos de HTTP; os 50 segundos padrão deixam folga.
- Vale para as rotas `/send/*` que escrevem a um contato, grupo ou canal. Fora da fila: Status (`/send/status/*`), votos de enquete, reações, edição, exclusão e marcar como lido.
- A fila é em memória: reiniciar o servidor descarta o que está esperando (as requisições HTTP em espera caem junto).
- Exemplo: `SEND_QUEUE_MIN_INTERVAL_MS=4000`, `SEND_QUEUE_MAX_INTERVAL_MS=9000` envia uma mensagem a cada 4 a 9 segundos por instância, no máximo cerca de 8 a 15 por minuto. **Não há valor "seguro" garantido**: o WhatsApp decide o que é abuso, e o ritmo ideal depende da idade da conta, do conteúdo e de quantos contatos já falaram com ela.

## Logs

- Cada instância grava em `LOG_DIRECTORY/<instanceId>/instance.log`; `GET /instance/logs/:instanceId?level=ERROR` (chave global) lê o arquivo.
- **Segredos não vão para o log.** Token de instância e chave global não são gravados, e a URL de um webhook aparece só como `esquema://host/…` (usuário, senha, caminho e query ficam de fora, porque costumam carregar token). O teste `TestNoSecretIsLoggedInTheSource` (`pkg/logger/secrets_scan_test.go`) varre o código atrás de linhas de log que gravam um segredo e falha se uma voltar.
