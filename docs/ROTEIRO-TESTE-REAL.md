# Roteiro de teste com conta real

Tudo o que foi corrigido passou nos testes automatizados, mas nada rodou com uma conta pareada. Marque cada item conforme testar.

Quando algo falhar, guarde três coisas: o passo, a resposta HTTP e o trecho do log (`GET /instance/logs/:instanceId?level=ERROR`, com a chave global, ou a pasta `LOG_DIRECTORY`).

Contexto do que foi mudado: [PROGRESSO.md](PROGRESSO.md).

## Preparo

- [ ] Subir o fork com Postgres e a licença do Evolution ativa (a API não inicia sem ela). Para compilar fora do Docker, use Go 1.26.
- [ ] Ter um segundo número para trocar mensagens e um grupo de teste em que a conta seja admin.
- [ ] Criar a instância com a chave global: `POST /instance/create`. Nos testes abaixo, `ADMIN` é a chave global e `KEY` é o token da instância (header `apikey`).

## 0. Básico (cobre o #115)

- [ ] `POST /instance/connect`, ler o QR em `GET /instance/qr` e parear. Esperado: conecta sem o erro 405 "Client outdated", e o log mostra `WhatsApp client version set to ...`.
- [ ] `GET /instance/status` mostra `connected` e `loggedIn` verdadeiros.
- [ ] `POST /send/text` para o segundo número: a mensagem chega. **Se não chegar, é o #115:** guarde a resposta e o log, é o que falta para investigar.

## 1. Conexões do Postgres (fase 1)

- [ ] Anotar: `SELECT datname, count(*) FROM pg_stat_activity WHERE datname IN ('evogo_auth','evogo_users') GROUP BY 1;`
- [ ] Fazer 10 vezes `POST /instance/reconnect` (ou `disconnect` e `connect`) e contar de novo. Esperado: o número de conexões não cresce a cada ciclo (antes subia cerca de 1 por reconexão).
- [ ] `POST /instance/forcereconnect/:id` (chave global, com `number`) também não acumula conexões.

## 2. Ciclo de vida (fase 3)

- [ ] `POST /instance/disconnect`, esperar 60 s: `GET /instance/status` continua desconectado (antes o cliente voltava sozinho).
- [ ] `POST /instance/connect` em seguida reconecta sem pedir novo QR.
- [ ] Dois `POST /instance/connect` quase ao mesmo tempo (`curl ... & curl ... &`): os dois respondem 200 e a instância segue normal (#75).
- [ ] QR timeout: numa instância nova, pedir o QR e não escanear. Esperado: depois do tempo limite a instância fica parada (log `QR timeout — stopping client`) até um novo `/instance/connect`. Essa é uma mudança de comportamento.
- [ ] Apagar com QR em andamento: `DELETE /instance/delete/:id` antes de escanear. Esperado: 200, sem panic no log do processo, e a instância não volta em `GET /instance/all` (#222).
- [ ] `DELETE /instance/logout` desloga; um novo `connect` gera QR.
- [ ] Presença (#55, #70): com `alwaysOnline=false`, enviar `POST /message/presence` com `{"number":"...","state":"composing"}`. Esperado: cerca de 20 s depois o celular volta a receber notificações. Com `alwaysOnline=true`, a conta continua online.
- [ ] Queda de rede: derrubar a internet do servidor por 30 a 60 s e restaurar. Esperado: o log mostra tentativas com intervalos crescentes e a instância reconecta sozinha, sem processo travado nem loop.
- [ ] Erro 463 (#50), só se acontecer: a resposta traz a explicação do bloqueio e, se o WhatsApp avisar, chega o evento `AccountReachoutTimelock` no webhook. Difícil de provocar de propósito.

## 3. Bugs de API (fase 4)

- [ ] `POST /user/avatar` e `POST /user/info` com `5511999999999` e com `+5511999999999`: respondem em poucos segundos (antes travavam 75 s).
- [ ] Carrossel (#51): `POST /send/carousel` com 3 cartões e botões `URL`, `CALL`, `COPY` (com `copyCode`) e `REPLY`. No celular, abrir o link, ligar e copiar o código funcionam.

## 4. Funcionalidades (fase 5)

- [ ] Enquete: `POST /send/poll`, votar `POST /send/pollVote` com `fromMe: true` e `selectedOptions` com o texto exato de uma opção. O voto aparece no aparelho do outro lado.
- [ ] `selectedOptions: []` remove o voto.
- [ ] Votar numa enquete criada por outra pessoa em grupo, passando `participant`. Votar numa enquete anterior ao pareamento deve dar erro explicativo (esperado).
- [ ] `POST /message/markplayed` com `{"number":"...","id":["ID-DO-AUDIO"]}` sobre um áudio recebido: o microfone fica azul para quem enviou.
- [ ] `POST /send/media` com `type: audio` e `ptt: false`: chega como arquivo de áudio com player. Sem `ptt` ou com `ptt: true`: continua como mensagem de voz. Testar também em multipart (`-F ptt=false`).

## 5. Correções da fase 6

- [ ] Arquivar e desarquivar uma conversa no celular (#95, #101): o processo não cai e o evento `Archive` chega (assine `ALL` ou `CHAT_PRESENCE`).
- [ ] WebSocket (#99): conectar em `ws://HOST/ws?token=KEY&instanceId=ID` com `websocketEnable: "enabled"` e disparar uns 20 `POST /send/text` em paralelo. Esperado: nenhum panic `concurrent write to websocket connection` e todos os eventos chegam.
- [ ] `POST /group/participant` com `{"groupJid":"...@g.us","action":"add","participants":["5511999999999"]}` responde 200 (#52, #97). Testar também `remove`, `promote` e `demote`.
- [ ] Eventos (#105, #193): conectar com `subscribe: ["ALL"]` e conferir que chegam `Picture` (o contato troca a foto), `UserAbout` (troca o recado) e `ButtonClick` (clique num botão). Com NATS, o mesmo vale para `NATS_GLOBAL_EVENTS`.
- [ ] `POST /instance/pair` numa instância já conectada responde 409 com `already_authenticated` (#231). Numa instância nova, devolve o código de pareamento.
- [ ] `POST /user/profileName` com `{"name":"Teste"}` responde em segundos e o nome muda no WhatsApp (#176). **Este é o que mais precisa de conta real.**
- [ ] Configurações avançadas (#111, #81): `PUT /instance/:id/advanced-settings` com só `{"alwaysOnline":true}` e depois `GET`: as outras opções continuam como estavam. Com o `apikey` de outra instância, 403. Com a chave global, 200.
- [ ] `POST /instance/connect` com corpo `{}`: depois, `GET /instance/info/:id` (chave global) ainda mostra o webhook e os eventos de antes. Para desligar um canal, enviar `"disabled"`.
- [ ] Com a instância deslogada, `POST /send/text` responde em menos de 1 s com `instance is not logged in` (#77).
- [ ] Citação (#189): `POST /send/text` com `quoted` contendo `messageId`, `participant` e `text`. O cartão da resposta mostra o texto citado e abre a mensagem ao tocar. Sem `text`, continua vazio como antes.

## 6. Fases 8 a 10 (2026-11-05)

Os itens de operação não precisam de conta pareada; os marcados com (conta) precisam.

- [ ] `GET /health` sem `apikey`: 200 com `status: ok` e os bancos `up`. Parar o Postgres por alguns segundos: 503 com `status: unavailable` e **sem** endereço ou usuário do banco no corpo.
- [ ] `GET /metrics` sem `apikey`: 401. Com a chave global: texto do Prometheus com `evogo_clients*`, `evogo_db_*` e os contadores. Fazer 10 vezes `POST /instance/reconnect` e conferir que `evogo_db_open_connections` não cresce e `evogo_instance_reconnects_total` sobe 10.
- [ ] Logs: `GET /instance/logs/:id` e a pasta `LOG_DIRECTORY` não podem conter o token da instância, a chave global nem o caminho/query de uma URL de webhook. Depois de apagar uma instância, procurar o token dela nos logs.
- [ ] Imagem (#104, #25) (conta): `POST /send/media` com uma imagem larga (por exemplo 1600x720) para o segundo número, com o celular com download automático desligado. O balão aparece já com a proporção certa, sem o quadrado cortado.
- [ ] Menção com documento (#114) (conta): em um grupo de teste em que a conta seja admin, `POST /send/media` com `type: document`, `caption` e `mentionAll: true`. Esperado: 200 (antes, 500) e todos mencionados. Repetir com imagem e vídeo com legenda.
- [ ] Edição (#92, #62) (conta): assinar `MESSAGE` e enviar uma mensagem do segundo número para a instância; editar a mensagem no celular. O webhook traz `data.messageAction` com `type: "edit"`, `targetMessageId` da mensagem original, `decrypted: true` e o texto novo em `message`; `data.IsEdit` é `true`. Se vier `decrypted: false`, guardar o campo `error` (provavelmente o segredo da mensagem original não estava guardado, por exemplo mensagem anterior ao pareamento).
- [ ] Exclusão (#92) (conta): apagar uma mensagem para todos no celular do segundo número. O webhook traz `data.messageAction` com `type: "revoke"` e o `targetMessageId`.
- [ ] Presença (#146) (conta): assinar `PRESENCE`, `POST /message/presence/subscribe` com `{"number":"…"}`. Esperado: `subscribed` com o contato e, em segundos, um evento `Presence`. Com `alwaysOnline=false` as mudanças seguintes param de chegar depois de uns 20 s; com `alwaysOnline=true` continuam chegando.
- [ ] Proxy (#123): `GET /instance/proxy/:id/status` (chave global) numa instância sem proxy: `configured: false`. Configurar um proxy válido e reconectar: `runtimeEnabled: true` e `lastAppliedAt` preenchido. Configurar um proxy com senha errada e reconectar: `fallbackWithoutProxy: true` e `runtimeEnabled: false`. Em nenhum caso a resposta traz usuário ou senha.
- [ ] Fila de envio (#236): com `SEND_QUEUE_MIN_INTERVAL_MS=3000`, `SEND_QUEUE_MAX_INTERVAL_MS=6000` (e **só com as duas instâncias de teste, poucas mensagens**), enviar 4 mensagens seguidas em paralelo: chegam espaçadas de 3 a 6 s e as respostas são as de sempre. Com `SEND_QUEUE_MAX_PENDING=1`, a terceira requisição simultânea responde 429 com `Retry-After`. `evogo_send_queue_*` aparecem em `/metrics`.
- [ ] Enquete sem Postgres de autenticação: subir com `POSTGRES_AUTH_DB` vazio. O servidor sobe (antes, caía na inicialização) e `GET /polls/:id/results` responde 501.

## Fora do roteiro

O passkey (#172), o nono dígito (#214), os botões e listas antigos (#170, #59, #71, #110) e o QR que não gera (#85, #148, #186) não foram corrigidos. Também ficaram de fora o resultado vazio de enquete (#60), a miniatura grande de link (#103), o carrossel em dois balões (#69) e o timer de mensagens temporárias (#79): o motivo de cada um está em [PROGRESSO.md](PROGRESSO.md), seção "O que falta".
