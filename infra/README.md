# Infra da EvoGo

Arquivos para publicar a imagem da EvoGo (fork `iichii1/evogo`) no registry local e usá-la no Portainer/Docker Swarm. Mesmo formato da pasta `infra` do Sys Stand; a EvoGo já roda num stack do Portainer, então aqui **só a imagem** é publicada: o stack existente muda apenas a linha `image:`.

## Imagem

Build e push (PowerShell, na raiz do repositório; o Docker precisa estar rodando):

```powershell
.\infra\scripts\publish-images.ps1 -ImagePrefix 192.168.100.30:5000/evogo -Tag 0.7.2-wf.1
```

A imagem fica em:

```text
192.168.100.30:5000/evogo/app:0.7.2-wf.1
```

Não use `registry.192.168.100.30:5000`; o prefixo correto já inclui o caminho do projeto.

- **Tag e versão são coisas diferentes.** A tag identifica a imagem (`0.7.2-wf.1` = base 0.7.2 do upstream + as correções do fork, 1ª imagem). A versão gravada no binário (`-X main.version`, `-Version` no script) vem do arquivo `VERSION` (`0.7.2`) e é o que o servidor informa ao serviço de licença. Mantenha a versão da base e mude só a tag a cada imagem nova.
- O `Dockerfile` continua na raiz do repositório (os exemplos de `docker/examples` e o stack de referência o usam), por isso não há `infra/docker/`.
- No fim do push o script imprime a imagem **com digest** (`...@sha256:...`), o endereço imutável que o Sys Stand usa para fixar o stack.
- `scripts/export-images.ps1` gera um `.tar` com a imagem, para levar a um servidor sem acesso ao registry (`docker load -i`).

## Portainer

No stack da EvoGo que já está no ar, troque só a linha da imagem pelo endereço acima e clique em **Update the stack**. Variáveis, volumes e rede não mudam. As novidades do fork (`/health`, `/metrics`, fila de envio e as demais) estão em [`docs/OPERACAO.md`](../docs/OPERACAO.md); a fila de envio vem **desligada**.

Depois do deploy, conferir (a licença continua a do banco):

```text
GET /server/ok            -> 200 (liveness)
GET /health               -> 200, bancos "up"
GET /metrics  (apikey)    -> texto do Prometheus
```

Para voltar à imagem anterior, troque a linha de volta e atualize o stack.
