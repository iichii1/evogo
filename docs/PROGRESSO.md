# Evogo: o que foi feito e o que falta

Documento de continuidade do fork `iichii1/evogo` (fork de `evolution-foundation/evolution-go`, base v0.7.2).
Escrito para quem continua o trabalho localmente, sem o histórico da conversa em que as mudanças foram feitas.

O roteiro para testar tudo com uma conta real está em [ROTEIRO-TESTE-REAL.md](ROTEIRO-TESTE-REAL.md).

## Como trabalhar neste repositório

**Requisitos**
- Go 1.26 ou mais novo (o `go.mod` exige).
- PostgreSQL. Há exemplos em `docker/examples/` (`.env.example` e compose).
- **Licença do Evolution ativa.** A API só sobe com ela. O gate de licença fica em `pkg/core`; não edite nem contorne esse pacote. O `pkg/core/c0.go` também não está formatado com `gofmt`; deixe como está.

**Comandos**

```bash
go build ./...
go vet ./...
go test -race -count=1 ./...      # é o que o CI roda
make swagger                      # regenera docs/swagger.*; precisa do swag v1.16.3
go install github.com/swaggo/swag/cmd/swag@v1.16.3
```

Os testes de integração têm a tag `integration` (por exemplo `pkg/whatsmeow/service/lifecycle_integration_test.go`) e precisam de um Postgres.

**Convenções usadas até aqui**
- Commits em português no formato `tipo(escopo): resumo` (`fix`, `feat`, `docs`, `test`, `ci`), com o motivo no corpo e o número da issue do upstream.
- Toda correção vem com um teste que falha no código antigo e passa no novo. A conferência foi feita à mão: reverter a correção, rodar o teste e ver falhar.
- Uma branch por fase, enviada ao GitHub e mergeada na `main` por fast-forward.
- O Swagger é gerado a partir das anotações dos handlers. O teste `pkg/routes/swagger_consistency_test.go` falha se uma rota ficar sem documentação ou se o Swagger citar uma rota inexistente.

**Peças de arquitetura que as correções criaram**
- `pkg/runtimestate`: `Map` genérico com trava por chave e `Session` com sinal de parada idempotente. Substitui os mapas soltos e o canal `killChannel`.
- `pkg/whatsmeow/service/whatsmeow.go`: arquivo central. Contém o container `sqlstore` compartilhado, a versão do WhatsApp Web aplicada no handshake, o ciclo de vida das instâncias (`StartClient`, `StopClient`, `ReconnectClient`, backoff) e o roteamento de eventos.
- `pkg/whatsmeow/service/event_groups.go`: `globalEventGroup`, a única tabela de evento para grupo de eventos (webhook, AMQP e NATS).

## O que foi feito

Todas as fases estão na `main`. Os branches `fix/fase*`, `feat/fase5-features` e `chore/fase7-ci-e-docs` continuam no GitHub e podem ser apagados.

### Fases 1 a 5 (PRs #1 a #5 no fork, mergeados)

| Fase | Issues do upstream | O que mudou |
|---|---|---|
| 1. Postgres | #106, #109, #112, #118, #165, #175, #215 | Cada reconexão criava um pool `sqlstore` novo e não fechava o anterior. Agora o container é criado uma vez e reaproveitado (50 reconexões: de 50 conexões para 1). Também corrigida uma injeção de SQL em `ForceUpdateJid` (o número agora vai como parâmetro). |
| 2. Erro 405 | #225, #228 | `whatsmeow` atualizado (2026-11-02) e a versão do WhatsApp Web aplicada no handshake, com valor de reserva se a busca falhar. Exige Go 1.26 e muda a resposta de bloqueio de contato. |
| 3. Ciclo de vida | #185, #211, #75, #55, #70, #222, #188, #50 (parcial) | Início, parada e reconexão serializados por instância; backoff nas reconexões; `connect` deixa de ser no-op com cliente morto; instância apagada não volta sozinha; presença volta a indisponível depois de "digitando"; aviso e evento `AccountReachoutTimelock` para o erro 463. |
| 4. Bugs de API | #76, #223, #51 | `/user/avatar` e `/user/info` usam o JID sem o "+" (antes travavam 75 s). Botões URL, CALL e COPY do carrossel deixaram de perder os parâmetros. |
| 5. Funcionalidades | #26, #217 (#45 já existia) | `POST /send/pollVote`; campo `ptt` em `/send/media` (`false` envia áudio comum com player). |

### Fase 6 (9 commits)

| Issues | Problema | Correção |
|---|---|---|
| #95, #101 | Panic ao arquivar conversa derrubava o processo | Payload do evento `Archive` montado direto do evento |
| #99 | Panic de escrita concorrente no websocket | Trava de escrita por conexão; envio fora do lock de cadastro; conexão antiga não remove a nova |
| #52, #97 | `/group/participant` sempre 400 | `ValidateJIDFields` aceita lista de strings |
| #105, #193 | Eventos de passkey, `PICTURE`, `USER_ABOUT`, `BUTTON_CLICK` não chegavam | `globalEventGroup` única; novo grupo `PASSKEY`, incluído em `ALL` |
| #231 | `/instance/pair` dava 500 em instância já autenticada | 409 com `code: already_authenticated` |
| #176 | `/user/profileName` travava | Nome enviado por app state (`BuildSettingPushName`), com timeout |
| #81, #111 | `advanced-settings` zerava campos, aceitava o ID de outra instância e recusava a chave global; `connect` zerava a configuração | Atualização parcial; conferência do dono; chave global aceita (`AuthInstanceOrAdmin`); `connect` mantém o que não veio no corpo |
| #77 | `/send/text` levava mais de 1 min em instância deslogada | Falha na hora com `ErrInstanceNotLoggedIn` |
| #189 | Citação aparecia vazia | Campo opcional `quoted.text` |

Já resolvidos pelas fases anteriores: #20, #21, #203.

### Fase 7 (3 commits)

- **CI:** `.github/workflows/ci.yml` roda `go build`, `go vet` e `go test -race` em todo PR e em todo push na `main`. O primeiro run no GitHub passou.
- **Swagger:** regenerado com o swag v1.16.3. O `make swagger` estava quebrado (faltavam `--parseDependency` e `--parseInternal`) e foi corrigido. As rotas `/license/*` são documentadas por funções só de anotação em `pkg/routes/license_swagger.go`.
- **Teste de rotas e Swagger:** `pkg/routes/swagger_consistency_test.go`.

### Mudanças de comportamento a conhecer

- Depois de um timeout de QR, a instância fica parada até um novo `/instance/connect`. Antes gerava QR de novo para sempre.
- `/instance/disconnect` agora desconecta de fato; antes o cliente voltava logo em seguida.
- `/instance/connect` mantém webhook, RabbitMQ, NATS, WebSocket e eventos que não vieram no corpo. Para desligar um canal, envie `"disabled"`.
- O token de uma instância só acessa as `advanced-settings` dela mesma (antes acessava as de qualquer uma).
- O `PUT /instance/:id/advanced-settings` é parcial: campo omitido mantém o valor atual.
- `ALL` passa a incluir o grupo `PASSKEY`.

### O que NÃO foi validado com WhatsApp real

Tudo passou nos testes automatizados, mas nada rodou com uma conta pareada. A API exige licença, e o teste real do ciclo de vida foi só até a etapa de QR. Os dois itens que mais precisam de conta real são `/user/profileName` (#176) e o cartão de citação (#189). Veja o roteiro de teste.

## O que falta

### A. Decisões suas

1. **Workflow "Build and Publish Docker Image":** falha a cada push na `main` do fork, no passo "Login to Docker Hub", porque publica em `evoapicloud/evolution-go` e o fork não tem as credenciais. Opções: (a) desativar o workflow no fork; (b) trocar a imagem de destino para a sua conta e configurar os segredos; (c) fazer o workflow rodar só no repositório original (`if: github.repository == 'evolution-foundation/evolution-go'`).
2. **Binário de 64 MB (`evolution-go`, na raiz):** está versionado desde os commits de sincronização com o upstream. Decidir se remove do repositório. Não mexi.
3. **Branches antigos no GitHub:** apagar ou manter.

### B. Ponto 2: operação e segurança (não dependem de conta real)

- **Health check de verdade:** o `/server/ok` só diz que o processo está de pé. Criar um endpoint que confira o Postgres e o número de instâncias conectadas.
- **Métricas:** instâncias online, reconexões, conexões do pool (`sql.DB.Stats()`), falhas de webhook. Os bugs mais graves do upstream (vazamento de pool, loop de reconexão) só aparecem tarde por falta disso.
- **Token em log:** `ClearInstanceCache` grava o token da instância no log (`Clearing instance cache - Token: %s`). Mascarar, e varrer o resto do código por casos parecidos.
- **Comparação da chave global:** o `AuthAdmin` usa `!=` comum; a nova `AuthInstanceOrAdmin` já usa comparação em tempo constante. Igualar.
- **Documentar o `/server/ok` no Swagger** (hoje está na lista de exceções do teste de rotas).

### C. Ponto 3: pedidos do upstream que ainda não foram tocados

Só li os títulos; ler o código e a issue antes de afirmar qualquer coisa.

- **Bugs prováveis:** #60 (`/polls/{id}/results` sempre 404), #69 (carrossel sai em dois balões), #114 (`mentionAll` com mídia), #92 e #62 (webhook de edição e exclusão com dados faltando).
- **Imagem e link:** #104, #25 (largura, altura e miniatura da imagem) e #103 (miniatura grande no link).
- **Funcionalidades:** #146 (expor `SubscribePresence` e decifrar mensagens editadas), #123 (configuração de proxy e status em runtime), #113 e #98 (modo só-admins no grupo; a rota `/group/settings` já existe, falta ver se cobre isso) e #79 (respeitar o timer de mensagens temporárias).

### D. Ponto 4 (ideia, só se você quiser): fila de envio com limite por instância

O #236 pede envio em lote, e o erro 463 mostra o risco de disparar rápido demais. Uma fila com limite e intervalo variável por instância protegeria a conta. É decisão de produto.

### E. Achados ainda não tratados

- **Evento `SendStatus`:** é emitido por `send_service.go` e não pertence a nenhum grupo em `globalEventGroup`, então provavelmente é descartado pelo filtro do webhook mesmo com `ALL` (o `ALL` é expandido no `connect`). Verificar.
- **`Disconnect` limpa `instance.Events`** quando a instância estava logada. Não ficou claro se é intencional; relacionado ao #111.
- **`OfflineSyncCompleted`** é emitido sem grupo (está numa lista de exceções no teste de eventos).
- **Status HTTP dos erros de envio:** o handler devolve 500 para quase tudo, inclusive `ErrInstanceNotLoggedIn`. Um 409 ou 400 seria mais correto.

### F. Dependem de conta real ou de decisão

- **#115** (`/send/text` não entrega): precisa de logs e de uma conta pareada. Fazer o teste 0 do roteiro e guardar a resposta e o log.
- **#172** (passkey): a issue só tem um print. O diretório `passkey-helper` pode estar ligado a ela.
- **#50** (erro 463): só a parte do aviso foi tratada; o bloqueio da conta não tem correção do nosso lado.
- **#214** (nono dígito brasileiro): `formatBRNumber` remove o 9 de propósito para DDD ≥ 31. Só dá para saber qual forma entrega com conta real.
- **Botões e listas recusados pelo WhatsApp** (#170, #59, #71, #110, #204, #219): dá para trocar para a mensagem interativa, mas só no celular se vê se renderiza.
- **QR que não gera** (#85, #148, #186), **#124** (salt do NCT em contas antigas), **#32** (número fixo), **#72** (arquivar com LTHash mismatch em Android).

### G. Resto do upstream

Dezenas de issues abertas do upstream continuam fora de qualquer fase (eram 72 em 04/11/2026). A lista atual está em https://github.com/evolution-foundation/evolution-go/issues.

## Referências

- Plano de trabalho no Claude Docs (com o histórico por fase): https://claude.ai/artifact/31VvLChyZxM8pUQRHPyeiM
- PRs das fases 1 a 5: https://github.com/iichii1/evogo/pulls?q=is%3Apr
