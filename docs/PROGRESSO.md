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
go test -race -count=1 ./...
make swagger                      # regenera docs/swagger.*; precisa do swag v1.16.3
go install github.com/swaggo/swag/cmd/swag@v1.16.3
```

**No Windows** não há compilador C, e o `-race` e o pacote `webp` exigem cgo (sem ele, `pkg/routes` e `pkg/sendMessage` nem compilam; mais quatro testes de `pkg/whatsmeow/service` falham ao limpar arquivos abertos). Rode no Linux pelo Docker. Uma imagem com `ffmpeg` (os testes de áudio só rodam com ele):

```bash
printf 'FROM golang:1.26\nRUN apt-get update -qq && apt-get install -y -qq --no-install-recommends ffmpeg ca-certificates && rm -rf /var/lib/apt/lists/*\n' | docker build -t evogo-ci:1.26 -
docker run --rm -v "$PWD:/src" -v evogo-gomod:/go/pkg/mod -v evogo-gobuild:/root/.cache/go-build -w /src evogo-ci:1.26 \
  sh -c 'go build -buildvcs=false ./... && go vet ./... && go test -race -count=1 ./...'
```

Os volumes nomeados guardam o cache; a primeira rodada baixa os módulos (cerca de 7 minutos), as seguintes são rápidas. Para regenerar o Swagger no mesmo contêiner: `go install github.com/swaggo/swag/cmd/swag@v1.16.3` e `swag init -g cmd/evolution-go/main.go -o ./docs --parseDependency --parseInternal`. O `gofmt -l` aponta os arquivos com fim de linha CRLF do Windows; confira com `tr -d '\r' < arquivo | gofmt -l`.

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

Todas as fases estão na `main`. As fases 8 a 10 (2026-11-05) foram validadas com `go build`, `go vet` e `go test -race` em Linux, na máquina local, e entraram na `main` por fast-forward, a partir das branches empilhadas `feat/fase8-operacao-e-seguranca`, `feat/fase9-pedidos-do-upstream` e `feat/fase10-fila-de-envio`. Os branches `fix/fase*`, `feat/fase5-features`, `chore/fase7-ci-e-docs` e `feat/fase8…10` continuam no GitHub e podem ser apagados.

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

- **CI:** `.github/workflows/ci.yml` roda `go build`, `go vet` e `go test -race` em todo PR e em todo push na `main`. **Não usamos CI do GitHub** (decisão do proprietário): os testes rodam só na máquina local, e o resultado do workflow não vale como validação nem precisa ser conferido.
- **Swagger:** regenerado com o swag v1.16.3. O `make swagger` estava quebrado (faltavam `--parseDependency` e `--parseInternal`) e foi corrigido. As rotas `/license/*` são documentadas por funções só de anotação em `pkg/routes/license_swagger.go`.
- **Teste de rotas e Swagger:** `pkg/routes/swagger_consistency_test.go`.

### Fase 8: operação e segurança (ponto 2)

Detalhes de uso em [OPERACAO.md](OPERACAO.md).

| Item | O que mudou |
|---|---|
| Health check | `GET /health` (público) confere cada banco (3 s no máximo) e conta os clientes em memória, conectados e logados. 503 se um banco não responde, sem repetir o erro do driver. O gate de licença já deixava `/health` passar. `/server/ok` continua só de liveness. |
| Métricas | `GET /metrics` (chave global), formato de texto do Prometheus, **sem dependência nova** (`pkg/metrics`): clientes, pools (`sql.DB.Stats()`), reconexões, quedas inesperadas e webhooks (entregues, tentativas que falharam, descartados). |
| Token em log | `ClearInstanceCache` e três mensagens "UserInfo cache cleared" gravavam o token da instância; o `/ws` gravava o token recebido quando a chave global estava errada (um erro de digitação registrava a chave quase inteira). Corrigidos, e o teste `TestNoSecretIsLoggedInTheSource` varre o código atrás de linhas de log que gravam token, chave ou senha. A varredura é por linha: não pega chamada de log em várias linhas. |
| URL de webhook em log | O produtor de webhook gravava a URL inteira, e o erro do cliente HTTP repete a URL com a query. Agora o log mostra só `esquema://host/…`. |
| Chave global | `AuthAdmin` e `/ws` passam a comparar em tempo constante (`MatchesGlobalKey`), como a `AuthInstanceOrAdmin` já fazia. Este conserto não muda comportamento visível e por isso não tem teste que falhe antes: os testes cobrem o comportamento (chave certa, errada, vazia, sem chave configurada), não o tempo. |
| Swagger | `/server/ok`, `/health` e `/metrics` documentados; `/server/ok` saiu da lista de exceções do teste de rotas. |

### Fase 9: pedidos do upstream (ponto 3)

Cada item foi lido na issue **e no código** antes de mexer. O que não se confirmou no código ficou de fora e está registrado em "O que falta".

| Issues | Achado | Correção |
|---|---|---|
| #104, #25 | A `ImageMessage` do `/send/media` já levava miniatura JPEG (desde a 0.7.0), mas não `Width`/`Height`, que o WhatsApp usa para desenhar o balão antes de baixar a mídia. | `newImageMessage` monta a mensagem (arquivo, URL, status, canal) com dimensões lidas por `image.DecodeConfig`; o cartão de imagem do carrossel também. Falta ver num aparelho se o quadrado cortado some. |
| #114 | Documento **com legenda** vai em `DocumentWithCaptionMessage`, mas o código das menções (`mentionAll` e `mentionedJid`) lia `msg.DocumentMessage` (nulo): ponteiro nulo e 500. Para imagem e vídeo com legenda o código estava certo. | `setMentions`/`mentionContextInfo` tratam os dois formatos; os dois `switch` de 50 linhas viraram uma chamada. |
| #92, #62, #146 (edição) | A edição chega como `secretEncryptedMessage` cifrado, com `IsEdit: false`; a exclusão, só como `protocolMessage.type: 0`. | O webhook de `Message` ganha `data.messageAction` (campo novo): `{"type":"edit","targetMessageId":"…","decrypted":true,"message":{…}}` e `{"type":"revoke","targetMessageId":"…"}`. A edição é decifrada com `DecryptSecretEncryptedMessage` e `IsEdit` vira `true`. Se não decifra (segredo da mensagem original ausente), sai `decrypted:false` com o motivo. Só foi testado com um decifrador falso: a decifragem real precisa de conta (roteiro). |
| #146 (presença) | `SubscribePresence` nunca era chamado, então o evento `Presence` não disparava para contatos. | `POST /message/presence/subscribe` (`number` ou `numbers`, até 50): fica online, assina cada contato e devolve `subscribed`/`failed`. Com `alwaysOnline=false` a instância volta a ficar indisponível pouco depois, como depois de um "digitando"; para acompanhar mudanças seguintes é preciso `alwaysOnline=true`. |
| #123 | `GET /instance/info` devolve `proxy` sempre vazio (de propósito) e `POST /instance/proxy` só prova que a configuração foi gravada; o cliente pode ter caído para a conexão direta. | `GET /instance/proxy/{id}/status` (chave global): `configured`, `clientStarted`, `runtimeEnabled`, `protocol`, `host`, `port`, `hasAuth`, `fallbackWithoutProxy`, `lastAppliedAt`, `lastError`. O cliente registra se aplicou o proxy, se o descartou ou se caiu para a conexão direta. Nunca devolve usuário, senha nem URL. |
| #113, #98 | Já resolvido: `POST /group/settings` existe e aceita `announcement`, `not_announcement`, `locked` e `unlocked` (e `approval_*`, `admin_add`, `all_member_add`). As issues eram de versões anteriores. | Nada a fazer. |
| #60 (parcial) | Não achei, só lendo o código, por que o resultado de enquete sairia sempre vazio num servidor com PostgreSQL. Achei outro defeito no mesmo caminho: sem `POSTGRES_AUTH_DB` (modo SQLite) o banco é nulo e `NewPollService` derrubava o servidor na inicialização com ponteiro nulo. | O serviço sobe sem a tabela; `GET /polls/{id}/results` responde 501 explicando o motivo, em vez de um 404 "sem votos". **O #60 continua aberto.** |

### Fase 10: fila de envio por instância (ponto 4)

`pkg/sendqueue` espaça as mensagens de cada instância com intervalo sorteado entre um mínimo e um máximo (#236, #50). **Desligada por padrão** (`SEND_QUEUE_MIN_INTERVAL_MS=0`). Fila cheia ou espera longa demais respondem 429 com `Retry-After`. Variáveis, comportamento e limites em [OPERACAO.md](OPERACAO.md). O limitador tem testes de ordem, isolamento entre instâncias, recusa, cancelamento e concorrência; a ligação com o `SendMessage` (o `Acquire` antes de simular a digitação e enviar) não tem teste porque o `SendMessage` exige um cliente do WhatsApp conectado. Os padrões (100 mensagens na fila, 50 s de espera) e o 429 são escolhas de implementação dentro do que o item D pedia ("limite e intervalo variável por instância"); podem ser ajustados.

### Fase 11: confiabilidade (2026-11-05)

Pedido do proprietário: implementar os recursos do WhatsApp que a análise contra a whatsmeow encontrou, com a confiabilidade primeiro. A lista completa, com o que ficou de fora, está em `docs/whatsapp-recursos-evogo.md` do Whatsflow.

| Item | O que mudou |
|---|---|
| Reenvio pelo celular | `configureClient` liga `AutomaticMessageRerequestFromPhone`: a mensagem que não decifra é pedida ao celular uma vez, 5 s depois da falha, e a resposta chega como um `Message` comum, com o mesmo ID. |
| Mensagem não decifrada | Novo evento `UndecryptableMessage`, no grupo `MESSAGE`. O caso de visualização única continua saindo como `Message`, como antes. |
| Mídia expirada | `POST /message/downloadmedia` aceita `info` (`id`, `chat`, `sender` em grupos, `fromMe`). Com ela, um 404 ou 410 faz o pedido de reenvio ao aparelho e a requisição espera até 30 s. Respostas novas: 410 `media_gone`, 504 `media_retry_timeout` e 400 `invalid_media_info`. Sem `info`, nada muda. |
| Eventos do celular | Grupo novo `CHAT_SYNC`, incluído em `ALL`: `MarkChatAsRead`, `DeleteChat`, `ClearChat`, `DeleteForMe`, `Mute`, `Pin`, `Star`, `UserStatusMute`, `Blocklist`, `PrivacySettings`, `UnarchiveChatsSetting` e também `Archive`. Antes caíam no `default` do handler e eram descartados. |
| Saúde da conexão | Em `CONNECTION`: `ClientOutdated`, `StreamError` (só o código), `KeepAliveTimeout`, `KeepAliveRestored`, `AppStateSyncError` e `CATRefreshError` (com o texto do erro), `OfflineSyncPreview` e `OfflineSyncCompleted`, que antes não tinha grupo. |
| Outros eventos | `CallReject`, `CallPreAccept` e `CallTransport` em `CALL`; `BusinessName` em `CONTACT`. |
| Filtro do webhook | O `default` de `CallWebhook` passa a consultar `globalEventGroup`, para a lista do webhook não ficar para trás da tabela das filas globais. |

Os testes falham ao desfazer cada correção (conferido à mão: a opção do cliente, o filtro do webhook e a condição do reenvio). A resposta do celular é testada com a cifra real da whatsmeow, mas o pedido em si só um aparelho prova: seção 7 do roteiro.

### Fase 12: consultas e ações prontas na whatsmeow (2026-11-05)

| Rota | O que faz |
|---|---|
| `POST /group/requests`, `/group/requests/update` | Lista, aprova e recusa pedidos de entrada. O serviço existia sem rota. |
| `POST /group/invite/link`, `/group/invite/info`, `/group/invite/join` | Prévia pelo link, prévia e aceite do cartão de convite recebido. |
| `POST /user/business` | Perfil comercial do contato. |
| `POST /chat/markread`, `/chat/markunread`, `/chat/delete` | Marca a conversa inteira e apaga, em todos os aparelhos. A última mensagem é opcional. |
| `POST /chat/star`, `/chat/unstar` | Favorita e desfavorita uma mensagem. |
| `POST /chat/disappearing`, `/user/disappearing` | Mensagens temporárias da conversa e padrão da conta: `off`, `24h`, `7d` ou `90d`. |
| `POST /message/order` | Itens de um pedido recebido. |

**#79:** `/send/text`, `link`, `media` (JSON e multipart), `poll`, `sticker`, `location` e `contact` aceitam `expiration`, em segundos. A whatsmeow não guarda o tempo de cada conversa, então a escolha de desenho foi o chamador informar: ele já recebe esse valor em `contextInfo.expiration` e em `/group/info`. Botão, lista e carrossel continuam sem o campo.

**`/chat/pin`, `archive`, `mute` com número solto:** `utils.ParseJID` acrescenta um `+` ao número, e esse JID ia direto para o índice do app state. Pode ser a causa do "TODO: not working" em `routes.go`, mas só o ensaio confirma; com JID completo o problema não existia. Agora as rotas de `/chat` usam `canonicalJID`, e um teste impede a volta de `utils.ParseJID` nesse arquivo.

**Conserto durante a fase:** o `expiration` chegou primeiro a 3 dos 8 envios, porque os outros passam por funções `...WithRetry`, e nenhum teste acusou. `TestEverySendPassesExpirationAlong` confere a ligação no código e falhou antes do conserto.

### Correção do ensaio de 2026-11-06

`/group/create` e `/group/participant` expiravam com `info query timed out`: o "+" de `utils.ParseJID` chegava ao servidor em nó bruto. As duas passam a ler as pessoas com `parseRequestParticipants`. Teste de ligação em `group_participants_wiring_test.go`; o comportamento real ainda não foi conferido com conta.

**Token no log:** a conexão da instância registrava a estrutura inteira (`%+v`), com token, QR, webhook e proxy. `Instance` ganhou `String()` que só mostra id, nome e conexão, e `TestPrintingAnInstanceHidesItsSecrets` falhou antes do conserto. Logs já gravados continuam com o token; só recriar a instância o troca.

**App state fora de sincronia:** depois de alguns patches, a coleção `regular_high` passou a falhar em todo patch com `failed to verify patch v28: mismatching LTHash`, e o que o celular fazia (favoritar numa conversa) nunca chegava. O whatsmeow só avisa. Agora o `AppStateSyncError` de hash diferente numa sincronização incremental baixa a coleção inteira de novo, no máximo uma vez a cada 10 minutos por coleção (`app_state_resync.go`). **Só o aparelho prova:** o teste cobre a decisão e o intervalo, não o efeito.

**Instâncias paradas depois de reiniciar:** não é versão nem sessão perdida. A sessão fica no banco, e um `/instance/connect` depois do reinício conectou em 1 segundo, sem QR e sem 405. Com `CONNECT_ON_STARTUP=false`, ninguém inicia os clientes ao subir. Com `true`, voltam as instâncias com `connected = true` e o `CLIENT_NAME` do processo; o desligamento (SIGTERM) não muda esse valor. `TestConnectOnStartupBringsBackWhatWasConnected` (tag `integration`) cobre esse caminho. **Limite:** se `client.Connect()` falhar ao subir (rede fora), nada tenta de novo até alguém conectar.

### Fase 13: tipos de mensagem novos (2026-11-05)

| Rota | O que faz |
|---|---|
| `POST /send/contacts` | Até 50 contatos num só cartão. O limite é uma trava nossa, não do WhatsApp. Recusa quebra de linha nos campos, que acrescentaria linhas ao vCard. |
| `POST /send/status/text` | Passa a aceitar `backgroundColor`, `textColor` (`#RRGGBB` ou `#AARRGGBB`) e `font`. Sem eles, sai como antes. |
| `POST /message/pin`, `/message/unpin` | Fixa e solta uma mensagem para todos: 24 h, 7 dias (padrão) ou 30 dias. |
| `POST /send/groupInvite` | Cartão de convite. `code` e `inviteExpiration` vêm do `AddRequest` da resposta de `/group/participant`. Se o código de um link comum também funciona aqui, só o aparelho diz. |
| `POST /send/requestPhone` | Pede ao contato que compartilhe o telefone. |

Os novos envios aceitam `expiration` e `quoted`. O `SendContact` deixou de imprimir o vCard (nome e telefone) na saída padrão. Entrada inválida nas rotas de envio com `ErrInvalidSendInput` responde 400.

Estes formatos foram montados a partir do protobuf da whatsmeow, sem helper dela: só o aparelho prova que aparecem certo (seção 9 do roteiro). O que for recusado ou aparecer errado volta para "O que falta".

### Mudanças de comportamento a conhecer

- Depois de um timeout de QR, a instância fica parada até um novo `/instance/connect`. Antes gerava QR de novo para sempre.
- `/instance/disconnect` agora desconecta de fato; antes o cliente voltava logo em seguida.
- `/instance/connect` mantém webhook, RabbitMQ, NATS, WebSocket e eventos que não vieram no corpo. Para desligar um canal, envie `"disabled"`.
- O token de uma instância só acessa as `advanced-settings` dela mesma (antes acessava as de qualquer uma).
- O `PUT /instance/:id/advanced-settings` é parcial: campo omitido mantém o valor atual.
- `ALL` passa a incluir o grupo `PASSKEY`.
- (Fase 8) Logs de webhook mostram só `esquema://host/…`; token de instância e chave global não são mais gravados.
- (Fase 9) O webhook de `Message` ganha `data.messageAction` em edições e exclusões, e `IsEdit` passa a `true` nas edições decifradas. Quem lia `IsEdit` ou o envelope cifrado continua vendo os mesmos campos.
- (Fase 9) Imagens enviadas por `/send/media`, status e carrossel levam largura e altura.
- (Fase 9) `GET /polls/{id}/results` responde 501 num servidor sem `POSTGRES_AUTH_DB`. O servidor nesse modo deixa de cair na inicialização.
- (Fase 13) `/send/contact` não imprime mais o vCard na saída padrão.
- (Fase 12) `/chat/pin`, `unpin`, `archive`, `unarchive`, `mute` e `unmute` com número solto passam a gravar o JID sem o `+`.
- (Fase 11) Quem assina `CONNECTION`, `MESSAGE`, `CALL` ou `CONTACT` passa a receber os eventos novos dessas categorias e precisa ignorar o que não conhece. O Whatsflow já ignora.
- (Fase 11) Uma mensagem que não decifra gera um pedido ao celular da própria conta.
- (Fase 10) Com `SEND_QUEUE_MIN_INTERVAL_MS` maior que zero, as rotas `/send/*` podem demorar até `SEND_QUEUE_MAX_WAIT_SECONDS` e passam a responder 429 quando a fila enche.

### O que NÃO foi validado com WhatsApp real

Tudo passou nos testes automatizados, mas nada rodou com uma conta pareada. A API exige licença, e o teste real do ciclo de vida foi só até a etapa de QR. Os dois itens que mais precisam de conta real são `/user/profileName` (#176) e o cartão de citação (#189). Veja o roteiro de teste.

## O que falta

Atualizado em 2026-11-05: os pontos B, C e D foram tratados nas fases 8 a 10 (acima). Ficou o que as fases não resolveram, em C e em E a F.

### A. Decisões suas

1. **Workflow "Build and Publish Docker Image":** falha a cada push na `main` do fork, no passo "Login to Docker Hub", porque publica em `evoapicloud/evolution-go` e o fork não tem as credenciais. Como o GitHub não é usado para testar nem para publicar, o mais simples é (a). Opções: (a) desativar o workflow no fork; (b) trocar a imagem de destino para a sua conta e configurar os segredos; (c) fazer o workflow rodar só no repositório original (`if: github.repository == 'evolution-foundation/evolution-go'`).
2. **Binário de 64 MB (`evolution-go`, na raiz):** está versionado desde os commits de sincronização com o upstream. Decidir se remove do repositório. Não mexi.
3. **Branches antigos no GitHub:** apagar ou manter.

### B. Ponto 2: operação e segurança

Feito na fase 8. Resta, se quiser: alertas e painel em cima das métricas (nada disso está no repositório), e a varredura de logs só pega chamadas de log numa linha.

### C. Ponto 3: o que a fase 9 não resolveu

- **#60** (`/polls/{id}/results` vazio): não achei a causa no código. Para investigar: enviar uma enquete, votar do celular e olhar o log da instância (`GET /instance/logs/:id?level=ERROR`) atrás de `Failed to decrypt vote` e `Failed to save poll vote`. O voto só é guardado se `DecryptPollVote` conseguir o segredo da enquete, e as opções saem como hash SHA-256 (não como texto): `optionCounts` e `selectedOptions` são hashes, o que também é um problema de usabilidade da rota. Precisa de conta pareada.
- **#69** (carrossel em dois balões): o servidor **não** divide a mensagem. `body` e `footer` vão no mesmo `InteractiveMessage`, desde a 0.6.1. Se o celular desenha duas partes, é o cliente do WhatsApp; só se vê no aparelho, e não há o que corrigir no código sem saber qual formato o WhatsApp espera.
- **#103** (miniatura grande no link): `sendLinkWithRetry` coloca a imagem inteira no campo `JPEGThumbnail`, que deveria ter só uma miniatura, e não envia a imagem em alta qualidade (`MediaLinkThumbnail`, `ThumbnailDirectPath` etc.). O caminho certo é subir a imagem com `whatsmeow.MediaLinkThumbnail` e preencher esses campos, como o Baileys faz. **Não mexi:** é uma mudança no formato da mensagem e só um aparelho mostra se o cartão grande aparece. Também notei que `fetchLinkMetadata` sobrescreve `title`, `description` e `imgUrl` informados pelo chamador sempre que o texto tem uma URL.
- **Estado da edição de evento e de enquete:** `messageAction` já identifica `eventEdit`, `pollEdit` e `pollAddOption`, mas só a edição de mensagem foi pensada ponta a ponta.

### D. Ponto 4: fila de envio

Feito na fase 10. Falta decidir os valores para as contas reais (o código não sugere um ritmo "seguro": o WhatsApp decide o que é abuso).

### E. Achados ainda não tratados

- **Evento `SendStatus`:** é emitido por `send_service.go` e não pertence a nenhum grupo em `globalEventGroup`, então provavelmente é descartado pelo filtro do webhook mesmo com `ALL` (o `ALL` é expandido no `connect`). Verificar.
- **`Disconnect` limpa `instance.Events`** quando a instância estava logada. Não ficou claro se é intencional; relacionado ao #111.
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
