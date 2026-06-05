# Roadmap: Do TCC ao Produto

## Painel de Progresso

> Atualizado em **2026-06-03**. Status **verificado contra o código** (não apenas marcação manual de checkbox).
> **Legenda:** ✅ concluído · 🟡 parcial · ⬜ pendente

| Fase | Concluído | Progresso |
|------|-----------|-----------|
| Fase 1 — Estabilização Técnica | 7 ✅ de 8 | `▓▓▓▓▓▓▓░` |
| Fase 2 — Generalização do Domínio | 0 de 7 | `░░░░░░░` |
| Fase 3 — Experiência do Desenvolvedor | 0 de 6 | `░░░░░░` |
| Fase 4 — Observabilidade e SLAs | 0 de 6 | `░░░░░░` |

### ✅ Concluído (verificado)
- **Logging estruturado (`zerolog`)** _(Fase 1)_ — pacote `internal/logger` (`internal/logger/logger.go`); todos os `fmt.Printf("DEBUG ...")` removidos do código de produção; `middleware.RequestLogger` registrado em `cmd/api/main.go`. _(ver Changelog)_
- **WAL distribuído (Redis Streams)** _(Fase 1)_ — interface `wal.WAL` com backends `file` (default) e `redis` (`internal/wal/redis_wal.go`, consumer group + `XAUTOCLAIM`). Opt-in via `wal.backend: redis`; permite múltiplas instâncias da API sem perder a garantia de recuperação. Testes e2e validados contra Redis real. _(ver Changelog)_
- **Autenticação por API key** _(Fase 1)_ — middleware `middleware.APIKeyAuth` (comparação constant-time; header `X-API-Key` ou `Authorization: Bearer`), protegendo `logs`/`merkle`/`wal`/`stats`. Opt-in via `auth.enabled`; validação impede ligar sem keys. _(ver Changelog)_
- **Rate limiting** _(Fase 1)_ — `middleware.RateLimiter` agora **conectado** via `rate_limit` config (opt-in, por IP). _(ver Changelog)_
- **`DeleteLog` soft delete** _(Fase 1)_ — `DELETE /logs/:id` agora marca `deleted_at` (`collections.SoftDeleteLog`) em vez de no-op; documento e âncora na blockchain preservados; deletados escondidos das rotas de leitura. _(ver Changelog)_
- **Paginação por cursor** _(Fase 1)_ — `GET /logs` aceita `cursor` (keyset por `created_at` + `id`) e retorna `next_cursor`; `offset` mantido (sem breaking change). Índice composto `(created_at, id)` adicionado. _(ver Changelog)_
- **Env vars p/ configs hardcoded do Fabric** _(Fase 1)_ — `peer_container`, `orderer_address`, `orderer_tls_ca_file`, `tls_enabled` viraram config/env (`FABRIC_*`); `client.go` não tem mais nomes/paths fixos. _(ver Changelog)_

### 🟡 Em andamento (último item da Fase 1)
- **Fabric via SDK** — **Fase A concluída**: transporte abstraído atrás de `fabric.Backend` (`internal/fabric/`), com `docker-exec` (default) selecionável por `fabric.transport`. **Fase B pendente**: backend `gateway` (gRPC), que exige bump de Go (≥1.20), a dep `fabric-gateway`, identidade/TLS do crypto-config e a rede rodando para validar; também corrige o nome de função do chaincode e remove o mount do `docker.sock`. _(ver Changelog)_

---

## Análise da Implementação

### O que foi construído

O TCC implementa um padrão de **armazenamento com ancora criptográfica em blockchain**: dados vão para MongoDB (rápido, consultável), são agrupados em Merkle Trees, e a raiz de cada batch é gravada no Hyperledger Fabric (imutável, auditável). Um WAL com `fsync` garante zero perda de dados antes de qualquer processamento.

Esse padrão — chamado de *Tamper-Evident Data Anchoring* — não é específico para logs. Ele se aplica a qualquer cenário onde a pergunta central é: **"esse dado pode ter sido alterado depois de gravado?"**

### Pontos fortes

- WAL com `O_SYNC` + `fsync` garante durabilidade antes de qualquer processamento
- Hash SHA-256 por registro + Merkle Tree por batch = prova matemática de integridade
- Separação clara entre camada quente (MongoDB, Redis) e camada de prova (Fabric)
- Worker pool com canal de jobs no batch processor — base sólida para escala horizontal
- API REST documentada com Swagger
- Suite de testes cobrindo 9 cenários de carga (10k a 1M registros, 100 a 10k/s)

### Problemas críticos para produção

| Problema | Impacto | Arquivo |
|----------|---------|---------|
| Fabric client usa `docker exec` em vez do SDK | Quebrável, não escalável, acoplamento ao runtime Docker | `fabric/client.go:64` |
| ~~Sem autenticação/autorização na API~~ ✅ **resolvido** | Auth por API key (`middleware.APIKeyAuth`, opt-in) nas rotas de dados | `internal/middleware/middleware.go` |
| ~~WAL é arquivo local~~ ✅ **resolvido** | Backend Redis Streams opt-in (`wal.backend: redis`) permite múltiplas instâncias | `internal/wal/redis_wal.go` |
| ~~`fmt.Printf("DEBUG ...")` em handlers~~ ✅ **resolvido** | Substituído por logging estruturado (`zerolog`) | `internal/logger/` |
| ~~Container names hardcoded (`peer0.org1.example.com`)~~ ✅ **resolvido** | Parametrizado por config/env (`FABRIC_PEER_CONTAINER` etc.) | `internal/fabric/client.go` |
| ~~`DeleteLog` é no-op silencioso~~ ✅ **resolvido** | Soft delete real (`deleted_at`), preservando documento e âncora | `internal/handlers/logs.go` |
| Rede Fabric de dev (1 org, 1 peer, 1 orderer) | Sem tolerância a falhas na blockchain | `fabric-network/` |
| ~~Paginação por offset~~ ✅ **resolvido** | Cursor keyset (`created_at`+`id`) opcional, com índice composto; `offset` mantido | `internal/handlers/logs.go` |

---

## Direção do Produto (decisão)

**Vertical escolhido: Compliance & Audit Trail.** O produto será posicionado primeiro para esse nicho de mercado — é onde o padrão *Tamper-Evident Data Anchoring* tem o encaixe mais direto e o menor esforço de adaptação (ver matriz de prioridade no fim do documento). As demais verticais (supply chain, notarização, IoT) ficam como expansão futura sobre a mesma base técnica.

Consequência prática para o roadmap: as features são priorizadas pelo que um time de compliance precisa para integrar sem entender Hyperledger Fabric — SDK simples, verificação de integridade e relatório de auditoria. A escala horizontal de infraestrutura (múltiplas instâncias, WAL distribuído) continua no roadmap técnico, mas atrás das entregas que destravam o primeiro cliente do vertical.

---

## Verticais com Potencial Comercial

O padrão implementado é valioso em qualquer mercado que exija **prova de integridade de dados ao longo do tempo**. As verticais mais promissoras:

### 1. Compliance & Audit Trail ✅ (vertical escolhido — melhor encaixe)
Regulações como LGPD, GDPR, SOX e HIPAA exigem provar que logs de acesso, transações e eventos não foram adulterados. O que foi construído resolve exatamente isso: um auditor pode recalcular o Merkle Root a partir dos dados no MongoDB e comparar com o que está na blockchain — qualquer adulteração é detectável matematicamente.

**Produto concreto:** SDK embarcável (Go/Java/Python) que empresas adicionam ao sistema existente para gerar trilha de auditoria à prova de adulteração.

### 2. Rastreabilidade de Supply Chain
Cada evento de custódia (fabricação → distribuição → entrega) é um registro com hash. A cadeia de hashes prova que a sequência de eventos não foi reescrita retroativamente.

### 3. Notarização de Documentos / Timestamping
Ancorar o hash de um documento na blockchain prova que ele existia naquele formato naquele momento. Equivalente digital de um cartório, sem intermediário.

### 4. Gerenciamento de Mudanças de Configuração (SRE/DevOps)
Cada mudança de infraestrutura (Terraform, Kubernetes manifests, feature flags) gravada com hash imutável. SOC 2 e ISO 27001 exigem esse tipo de controle.

### 5. Dados IoT com Integridade Garantida
Sensores industriais, equipamentos médicos — dados que precisam ser auditáveis e não alteráveis para fins regulatórios.

---

## Roadmap de Produto

### Fase 1 — Estabilização Técnica (2-3 meses) — `7 ✅ de 8`

**Objetivo:** Tornar o código executável em ambiente real, não só em dev.

- [ ] Substituir `docker exec` no `FabricClient` pela integração real com `fabric-sdk-go` (o SDK já está no `go.mod` mas não é usado)
- [x] Remover todos os `fmt.Printf("DEBUG ...")` do código de produção — substituído por **structured logging com `zerolog`** (escolhido em vez de `slog` por compatibilidade com Go 1.18 e por ser zero-alocação no hot-path). Novo pacote `internal/logger`; logs em JSON com `request_id`, `service`, `caller`. _(ver changelog)_
- [x] **Autenticação por API key** — middleware `middleware.APIKeyAuth` em `internal/middleware/` (comparação constant-time; `X-API-Key` ou `Authorization: Bearer`). Protege `logs`/`merkle`/`wal`/`stats`; opt-in via `auth.enabled` + `auth.api_keys`. Keys em banco / por-tenant ficam para a Fase 2 (multi-tenancy). _(ver changelog)_
- [x] **WAL distribuído** — backend Redis Streams (consumer group) opt-in via `wal.backend: redis`; default continua `file`. Interface `wal.WAL` + `RedisWAL`/`WriteAheadLog`(file)/`NoopWAL`; recuperação de instância morta via `XAUTOCLAIM`. Redis em modo AOF (`appendfsync always`) para paridade de durabilidade. _(ver changelog)_
- [x] **Paginação por cursor** — `GET /logs?cursor=` (keyset por `created_at` desc + `id` desc; cursor opaco base64) retornando `next_cursor`; `offset` mantido (aditivo, sem breaking change). Índice composto `(created_at:-1, id:-1)`. Validado por `database.TestKeysetPagination` (sem overlap/lacuna, com empate no mesmo ms). _(ver changelog)_
- [x] **`DeleteLog` soft delete** — `collections.SoftDeleteLog` marca `deleted_at` (via `$currentDate`); documento e âncora na blockchain preservados (não entra no `CalculateHash`). Deletados escondidos de `GET /logs` e `GET /logs/:id`; idempotente. _(ver changelog)_
- [x] **Env vars para configs hardcoded do Fabric** — `peer_container`, `orderer_address`, `orderer_tls_ca_file`, `tls_enabled` na `FabricConfig` (env `FABRIC_*`), com validação. `client.go` constrói os args do `docker exec` a partir da config (`invokeArgs`/`queryArgs`, testáveis); zero nomes/paths fixos. _(ver changelog)_
- [x] **Rate limiting** — `middleware.RateLimiter` conectado em `cmd/api/main.go` via config `rate_limit` (opt-in, por IP, com env overrides). _Nota:_ in-memory por instância; um limiter compartilhado em Redis é o follow-up para multi-instância. _(ver changelog)_

### Fase 2 — Generalização do Domínio (2-3 meses) — `0 de 7 (não iniciada)`

**Objetivo:** Remover o acoplamento ao domínio de "logs" para suportar qualquer tipo de registro auditável.

- [ ] Abstrair `Log` para `Record` com schema flexível — campos obrigatórios: `id`, `timestamp`, `source`, `payload` (qualquer JSON), `hash`
- [ ] Tornar o `CalculateHash` configurável por schema: o cliente define quais campos entram no hash
- [ ] Renomear rotas de `/api/v1/logs` para `/api/v1/records` com namespace de domínio (`/api/v1/{domain}/records`)
- [ ] Multi-tenancy: cada cliente/organização tem seu próprio channel no Fabric e collection no MongoDB
- [ ] Configurar rede Fabric de produção: mínimo 3 orgs, consenso Raft com 3 orderers, CAs separadas
- [ ] Webhook/callback quando um batch é ancorado na blockchain (para integrações externas)
- [ ] SDK cliente em Go com suporte a retry automático e verificação de integridade local

### Fase 3 — Experiência do Desenvolvedor (2-3 meses) — `0 de 6 (não iniciada)`

**Objetivo:** Reduzir o tempo de integração de semanas para horas.

- [ ] SDK em Python (maior mercado de ciência de dados e compliance scripts)
- [ ] CLI para verificação offline: dado um arquivo CSV de registros, verifica se o Merkle Root bate com o da blockchain
- [ ] Dashboard web mínimo: listar batches, ver status de sincronização, verificar integridade
- [ ] Helm chart para deploy no Kubernetes (API + MongoDB + Fabric peer como sidecar)
- [ ] Documentação com guias por vertical: "Como usar para auditoria LGPD", "Como usar para supply chain"
- [ ] Sandbox público com rede Fabric de teste para desenvolvedores avaliarem

### Fase 4 — Observabilidade e SLAs (1-2 meses) — `0 de 6 (não iniciada)`

**Objetivo:** Suportar clientes em produção com garantias concretas.

- [ ] Métricas Prometheus já existem (`/api/v1/metrics`) — criar dashboards Grafana padronizados
- [ ] Alertas: batch pendente há mais de X minutos, discrepância de Merkle Root detectada, WAL acima de threshold
- [ ] API de verificação pública: endpoint sem autenticação que recebe um hash e retorna se ele está ancorado na blockchain (para auditores externos)
- [ ] Relatório de auditoria em PDF: dado um período, gera documento com todos os batches, raízes e TxIDs do Fabric
- [ ] SLA de latência P99 < 500ms para escrita com WAL habilitado
- [ ] Runbook de recuperação de desastres documentado

---

## Prioridade de Verticais por Esforço vs. Retorno

```
Alto retorno │  Supply Chain     Compliance/Audit ◄── começar aqui
             │  Notarização
             │
Baixo retorno│  IoT              Config Management
             └──────────────────────────────────────
                Baixo esforço        Alto esforço
                de adaptação         de adaptação
```

**Recomendação:** Compliance & Audit Trail é o melhor ponto de entrada. O produto já resolve o problema tecnicamente — o que falta é empacotar para que um time de compliance possa integrar sem entender Hyperledger Fabric. A Fase 1 + Fase 3 (especialmente o SDK Python e o relatório de auditoria) são o caminho mais direto para um primeiro cliente pagante.

---

## Referências de Mercado

- **Immudb** (Codenotary) — banco imutável com verificação criptográfica, foco em DevOps e compliance
- **Chainpoint** — ancoragem de hashes em Bitcoin/Ethereum, mais genérico
- **Hyperledger Fabric + Fabric Token SDK** — para casos financeiros regulados
- **LTO Network** — audit trail as a service com blockchain pública

O diferencial desta implementação em relação a esses produtos é o **WAL + zero data loss** combinado com **Hyperledger Fabric permissionada** (ideal para ambientes corporativos que não podem usar blockchain pública por questões de compliance).

---

## Changelog de Execução

### 2026-06-04 — Fabric via SDK, Fase A: abstração de transporte

Início do último item da Fase 1. Fase A prepara o terreno sem dep nova nem bump de Go.

- Interface `fabric.Backend` (`Invoke`/`Query`/`HealthCheck`/`Close`) em `internal/fabric/backend.go`; o transporte atual virou `dockerExecBackend` (`internal/fabric/docker_exec.go`).
- `FabricClient` agora delega para um `Backend` escolhido por `fabric.transport` (`docker-exec` default | `gateway`). `NewFabricClient` passou a retornar erro; `main.go` trata e chama `Close()` no shutdown.
- Config `fabric.transport` (+ `FABRIC_TRANSPORT`) com validação. Sem mudança de comportamento (default = docker-exec).
- Testes reorganizados: `client_test.go` (client/delegação, seleção de transporte) e `docker_exec_test.go` (args/extractTxID). `go build`/`vet`/`test ./...` limpos.
- **Achado:** o `client.go` chama funções (`storeMerkleBatch`/`getMerkleBatch`) que **não existem** no chaincode (`StoreMerkleRoot`/`QueryMerkleBatch`) — bug pré-existente, a ser corrigido na Fase B (com validação contra a rede). Marcado por comentário no código.

**Fase B (pendente, precisa de decisão):** bump de Go + `fabric-gateway`, identidade `Admin@org1`/TLS do crypto-config, `SubmitTransaction`/`EvaluateTransaction`, remover o mount do `docker.sock`.

### 2026-06-04 — Env vars para os configs hardcoded do Fabric

Sexta leva (Fase 1). Remove nomes de container e paths fixos do cliente Fabric.

- `FabricConfig` ganhou `peer_container`, `orderer_address`, `orderer_tls_ca_file` e `tls_enabled` (env `FABRIC_PEER_CONTAINER`, `FABRIC_ORDERER_ADDRESS`, `FABRIC_ORDERER_TLS_CA_FILE`, `FABRIC_TLS_ENABLED`), com defaults iguais ao comportamento atual e validação (sync ligado exige `peer_container`; TLS ligado exige cafile).
- `internal/fabric/client.go`: a construção do comando `docker exec` foi extraída para `invokeArgs`/`queryArgs`, montados a partir da config; `--tls`/`--cafile` só entram quando `tls_enabled`. `HealthCheck` e `GetStats` também usam `PeerContainer`. **Zero** `peer0.org1.example.com`/paths hardcoded.
- Testes: `fabric.TestInvokeArgsFromConfig`/`TestInvokeArgsNoTLS`/`TestQueryArgsFromConfig` e `config.TestValidateFabric`. `go build`/`vet`/`test ./...` limpos.
- _Obs.:_ ainda é `docker exec` por baixo — o último item da Fase 1 (Fabric via SDK) troca o transporte por gRPC e remove o mount do `docker.sock`. Este passo só desacopla os nomes/paths.

### 2026-06-03 — Paginação por cursor (keyset)

Quinta leva (Fase 1). Resolve a paginação por offset, que não escala para datasets grandes.

- `GET /logs` aceita `cursor` (opaco, base64) além de `offset`. **Aditivo, sem breaking change**: sem `cursor`, o comportamento de `offset` é idêntico.
- Keyset por `(created_at` desc`, id` desc`)`: o cursor codifica `created_at` (em ms) + `id` do último item; a próxima página filtra `created_at < c` OR (`created_at == c` AND `id < c.id`). Resposta inclui `next_cursor` quando a página vem cheia.
- Sort agora tem desempate por `id` (ordem estável). Novo índice composto `(created_at:-1, id:-1)` em `CreateIndexes`.
- `ListLogsResponse.NextCursor`; helpers `encodeCursor`/`decodeCursor` (`internal/handlers/cursor.go`); cache key separada para cursor (mesmo prefixo `logs:list:`, invalidada junto).
- Testes: `handlers.TestCursorRoundTrip`/`TestDecodeCursorInvalid` (unit) e `database.TestKeysetPagination` (contra Mongo real: sem overlap/lacuna, ordem estável, empate no mesmo ms). `go build`/`vet`/`test ./...` limpos.

**Próximos candidatos:** env vars para configs hardcoded do Fabric, Fabric via SDK (substituir `docker exec`).

### 2026-06-03 — DeleteLog com soft delete

Quarta leva (Fase 1). Resolve o `DeleteLog` no-op silencioso.

- Novo campo `Log.DeletedAt` (`*FlexTime`, `omitempty`), **fora** do `CalculateHash` — soft-deletar nunca invalida a prova de integridade nem a âncora on-chain.
- `collections.SoftDeleteLog(id)` marca `deleted_at` via `$currentDate` (só se ainda não deletado); retorna `mongo.ErrNoDocuments` se não existe. O documento permanece para verificação de batch.
- `DELETE /logs/:id` reescrito: era no-op (retornava 200 sem deletar), agora faz soft delete idempotente e invalida o cache. `GET /logs` e `GET /logs/:id` escondem deletados (`deleted_at` `$exists:false`); queries internas de batch/verificação continuam vendo tudo.
- Teste `database.TestSoftDeleteLog` (contra Mongo real): preserva documento, esconde de listagens, idempotência. `go build`/`vet`/`test ./...` limpos.

**Próximos candidatos:** paginação por cursor, env vars para configs hardcoded do Fabric, Fabric via SDK.

### 2026-06-03 — Segurança da API: autenticação + rate limiting

Terceira leva (Fase 1). Fecha o problema crítico "qualquer requisição é aceita".

**Autenticação por API key**
- Novo middleware `middleware.APIKeyAuth(headerName, keys)`: aceita a key no header configurável (default `X-API-Key`) ou como `Authorization: Bearer <key>`; comparação **constant-time** (`crypto/subtle`) para não vazar a key por timing.
- Protege as rotas de dados (`/logs`, `/merkle`, `/wal`, `/stats`); `/`, `/health` e `/swagger` ficam abertas. Aplicado por grupo em `cmd/api/main.go`.
- Config `auth` (`enabled`, `header_name`, `api_keys`) + env (`AUTH_ENABLED`, `AUTH_HEADER_NAME`, `AUTH_API_KEYS` separada por vírgula). **Opt-in** (default off); validação impede ligar sem keys (anti-lockout).
- Decisão: keys estáticas via config como MVP; keys em banco / por-tenant ficam para a Fase 2 (multi-tenancy).

**Rate limiting**
- `middleware.RateLimiter` (que existia mas não era usado) agora **conectado** em `cmd/api/main.go` via config `rate_limit` (`enabled`, `max_requests`, `window`) + env. Opt-in, por IP. Nota: in-memory por instância — limiter compartilhado em Redis é o follow-up para multi-instância.

**Qualidade**
- Novos testes: `internal/middleware/middleware_test.go` (auth válido/inválido/ausente, Bearer, rate limiter 429) e `pkg/config/config_test.go` (validação anti-lockout, rate limit, `splitAndTrim`).
- `go build`, `go vet`, `go test ./...` limpos.

**Próximos candidatos:** paginação por cursor, `DeleteLog` com soft delete, env vars para configs hardcoded do Fabric.

### 2026-06-03 — WAL distribuído (Redis Streams) + internacionalização

Segunda leva (Fase 1). Item escolhido: **WAL distribuído**, para destravar múltiplas instâncias da API.

**WAL distribuído**
- Introduzida a interface `wal.WAL` (pacote `internal/wal`). O WAL de arquivo existente (`O_SYNC` + `fsync`) vira o backend `file` e a satisfaz sem mudança de comportamento.
- Novo backend `RedisWAL` (`internal/wal/redis_wal.go`) sobre **Redis Streams + consumer group**: `XADD` no `Write`; processor com retry do próprio PEL (`XREADGROUP ... 0`), `XAUTOCLAIM` para reivindicar pendências de instâncias mortas, e `XREADGROUP ... >` bloqueante para entradas novas; `XACK` + `XDEL` após insert idempotente.
- Factory `wal.New(cfg, redisClient)` seleciona o backend por `wal.backend` (`file` | `redis`). **Default `file`** — deploys atuais não mudam; `redis` é opt-in. Backend `redis` sem Redis = erro explícito (sem rebaixar a durabilidade em silêncio). `NoopWAL` cobre o caso desabilitado.
- `cmd/api/main.go` e os handlers (`LogHandler`/`WALHandler`/`StatsHandler`) passam a usar a interface.
- Config `wal.backend` + settings de stream (`stream_key`, `consumer_group`, `read_count`, `block_timeout`, `claim_min_idle`) com env overrides e validação.
- **Durabilidade:** no modo `redis` depende da persistência do Redis. `docker-compose` do Redis ajustado para `--appendonly yes --appendfsync always` (paridade com o `fsync` do arquivo). Documentado em `config.yaml`/`.env.example`/README.
- Testes: factory (sempre roda) + e2e e retry-on-failure (validados contra Redis real). Removido código morto `NewWriteAheadLogWithConfig`.

**Internacionalização & saída formal (do TCC ao produto)**
- Código, comentários do chaincode, scripts do Fabric, README e os dois Makefiles traduzidos para inglês (ROADMAP e changelog permanecem em PT como docs internas).
- Saídas de console (scripts + Makefiles + banner de boot) formalizadas: emojis removidos, prefixos `[INFO]`/`[OK]`/`[WARNING]`/`[ERROR]`.

`go build`, `go vet` e `go test ./...` limpos.

**Próximos candidatos:** autenticação (JWT/API keys) + rate limiting real, paginação por cursor, `DeleteLog` com soft delete.

### 2026-06-03 — Fundação: logging estruturado + arquitetura

Primeira leva do roadmap (parte da Fase 1). Foco em base limpa antes de features.

**Logging estruturado (`zerolog`)**
- Novo pacote `internal/logger`: logger global configurável por nível, formato (`json`/`console`), output (stdout/stderr/arquivo) e `caller`. Helper `WithRequestID` para correlação de requisições.
- Removidos **todos** os `fmt.Printf("DEBUG ...")`/`Printf`/`Println` do código de produção (`handlers/logs.go`, `cache/redis.go`, `merkle/batch_processor.go`, `database/mongodb.go`, `middleware`). Os `fmt.Printf` que sobraram são apenas em CLIs de teste de carga (`tests/performance/*`), onde saída de console é o esperado.
- `middleware.Logger` (que usava `fmt.Printf`) virou `middleware.RequestLogger`, emitindo um evento estruturado por requisição (método, path, status, latência, client_ip, bytes, `request_id`) com nível derivado do status (4xx→warn, 5xx→error).
- Handler de criação de log não vaza mais o erro interno do banco na resposta HTTP; o detalhe vai para o log estruturado.

**Arquitetura**
- `cmd/api/main.go` reescrito: logger inicializado cedo (com logger de bootstrap para falhas pré-config), porta/host/timeouts vindos de `cfg.Server` (antes relia em `os.Getenv` solto), shutdown gracioso ordenado e sem os `TODO` pendentes.
- Removido código morto: `cmd/api/dependencies.go` (container de DI nunca usado) e os handlers legados `makeHealthHandler`/`makeStatsHandler`.
- Middlewares reordenados para `RequestID` → `RequestLogger` (request_id disponível no log da requisição).

**Qualidade**
- Corrigidos testes pré-existentes que não compilavam (`models.Log.CreatedAt` é `FlexTime`, não `time.Time`/`string`) em `database/mongodb_test.go` e `merkle/batch_processor_test.go`.
- Novo `internal/logger/logger_test.go`. `go build`, `go vet` e `go test ./...` limpos.

**Decisão de produto:** vertical de mercado = **Compliance & Audit Trail** (ver seção _Direção do Produto_).

**Próximos candidatos:** autenticação (JWT/API keys) + rate limiting real, integração Fabric via SDK (substituir `docker exec`), paginação por cursor.
