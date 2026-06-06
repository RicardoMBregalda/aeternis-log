# Roadmap: Do TCC ao Produto

## Painel de Progresso

> Atualizado em **2026-06-06**. Status **verificado contra o código** (não apenas marcação manual de checkbox).
> **Legenda:** ✅ concluído · 🟡 parcial · ⬜ pendente

| Fase | Concluído | Progresso |
|------|-----------|-----------|
| Fase 1 — Estabilização Técnica | 8 ✅ de 8 | `▓▓▓▓▓▓▓▓` |
| Fase 2 — Generalização do Domínio | 7 ✅ de 7 | `▓▓▓▓▓▓▓` |
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
- **Fabric via SDK (Gateway gRPC)** _(Fase 1)_ — transporte atrás de `fabric.Backend`: **`gateway` (default)** ou `docker-exec` (fallback), via `fabric.transport`. `gateway` usa `fabric-gateway` (gRPC+TLS, identidade X.509, **sem `docker.sock`**). Go 1.25 + `fabric-gateway` v1.11.0 (latest). ✅ **E2E validado contra a rede viva** (Submit `StoreMerkleRoot` + Evaluate `QueryMerkleBatch` reais, txID confirmado — `TestGatewayE2E`) **e** API subindo com gateway default (`/health` fabric healthy). _(ver Changelog)_

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
| ~~Fabric client usa `docker exec` em vez do SDK~~ ✅ **resolvido** | Backend `gateway` (Fabric Gateway gRPC) opt-in via `fabric.transport`; sem `docker.sock` | `internal/fabric/gateway.go` |
| ~~Sem autenticação/autorização na API~~ ✅ **resolvido** | Auth por API key (`middleware.APIKeyAuth`, opt-in) nas rotas de dados | `internal/middleware/middleware.go` |
| ~~WAL é arquivo local~~ ✅ **resolvido** | Backend Redis Streams opt-in (`wal.backend: redis`) permite múltiplas instâncias | `internal/wal/redis_wal.go` |
| ~~`fmt.Printf("DEBUG ...")` em handlers~~ ✅ **resolvido** | Substituído por logging estruturado (`zerolog`) | `internal/logger/` |
| ~~Container names hardcoded (`peer0.org1.example.com`)~~ ✅ **resolvido** | Parametrizado por config/env (`FABRIC_PEER_CONTAINER` etc.) | `internal/fabric/client.go` |
| ~~`DeleteLog` é no-op silencioso~~ ✅ **resolvido** | Soft delete real (`deleted_at`), preservando documento e âncora | `internal/handlers/logs.go` |
| ~~Rede Fabric de dev (1 org, 1 peer, 1 orderer)~~ ✅ **resolvido** | Rede staging multi-org: 3 peer orgs, Raft de 3 orderers (tolera 1 falha), CAs separadas, endosso `MAJORITY` 2/3 — stack paralela à dev | `hybrid-architecture/fabric-network/prod/` |
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

### Fase 1 — Estabilização Técnica (2-3 meses) — `8 ✅ de 8` ✅

**Objetivo:** Tornar o código executável em ambiente real, não só em dev.

- [x] **Fabric via SDK (Gateway gRPC)** — transporte atrás de `fabric.Backend`; novo `gatewayBackend` (`internal/fabric/gateway.go`) usa `github.com/hyperledger/fabric-gateway` (gRPC+TLS, identidade X.509 do MSP, `SubmitTransaction`/`EvaluateTransaction`), selecionável por `fabric.transport: gateway`. Go 1.25 + `fabric-gateway` v1.11.0; `docker.sock` dispensado nesse modo; nomes de função corrigidos (`StoreMerkleRoot`/`QueryMerkleBatch`). **E2E validado** (`TestGatewayE2E`, build tag `integration`). _(ver changelog)_
- [x] Remover todos os `fmt.Printf("DEBUG ...")` do código de produção — substituído por **structured logging com `zerolog`** (escolhido em vez de `slog` por compatibilidade com Go 1.18 e por ser zero-alocação no hot-path). Novo pacote `internal/logger`; logs em JSON com `request_id`, `service`, `caller`. _(ver changelog)_
- [x] **Autenticação por API key** — middleware `middleware.APIKeyAuth` em `internal/middleware/` (comparação constant-time; `X-API-Key` ou `Authorization: Bearer`). Protege `logs`/`merkle`/`wal`/`stats`; opt-in via `auth.enabled` + `auth.api_keys`. Keys em banco / por-tenant ficam para a Fase 2 (multi-tenancy). _(ver changelog)_
- [x] **WAL distribuído** — backend Redis Streams (consumer group) opt-in via `wal.backend: redis`; default continua `file`. Interface `wal.WAL` + `RedisWAL`/`WriteAheadLog`(file)/`NoopWAL`; recuperação de instância morta via `XAUTOCLAIM`. Redis em modo AOF (`appendfsync always`) para paridade de durabilidade. _(ver changelog)_
- [x] **Paginação por cursor** — `GET /logs?cursor=` (keyset por `created_at` desc + `id` desc; cursor opaco base64) retornando `next_cursor`; `offset` mantido (aditivo, sem breaking change). Índice composto `(created_at:-1, id:-1)`. Validado por `database.TestKeysetPagination` (sem overlap/lacuna, com empate no mesmo ms). _(ver changelog)_
- [x] **`DeleteLog` soft delete** — `collections.SoftDeleteLog` marca `deleted_at` (via `$currentDate`); documento e âncora na blockchain preservados (não entra no `CalculateHash`). Deletados escondidos de `GET /logs` e `GET /logs/:id`; idempotente. _(ver changelog)_
- [x] **Env vars para configs hardcoded do Fabric** — `peer_container`, `orderer_address`, `orderer_tls_ca_file`, `tls_enabled` na `FabricConfig` (env `FABRIC_*`), com validação. `client.go` constrói os args do `docker exec` a partir da config (`invokeArgs`/`queryArgs`, testáveis); zero nomes/paths fixos. _(ver changelog)_
- [x] **Rate limiting** — `middleware.RateLimiter` conectado em `cmd/api/main.go` via config `rate_limit` (opt-in, por IP, com env overrides). _Nota:_ in-memory por instância; um limiter compartilhado em Redis é o follow-up para multi-instância. _(ver changelog)_

### Fase 2 — Generalização do Domínio (2-3 meses) — `6 de 7`

**Objetivo:** Remover o acoplamento ao domínio de "logs" para suportar qualquer tipo de registro auditável.

- [x] **Abstrair `Log` para `Record`** — modelo genérico `models.Record` (`domain`, `id`, `timestamp`, `source`, `payload` JSON livre, `hash` + campos de auditoria); collection `records` escopada por domínio. _(ver changelog)_
- [x] **`CalculateHash` configurável** — hash SHA-256 sobre `id|timestamp|source|payload canônico`; `hash_fields` opcional escolhe quais chaves do payload entram no hash (guardado no record p/ reprodutibilidade); payload canônico com chaves ordenadas (independe da ordem). _(ver changelog)_
- [x] **Rotas `/api/v1/{domain}/records`** — CRUD genérico (create/list+cursor/get/delete soft) sob namespace de domínio, protegido pela auth. **Aditivo:** `/logs` mantido (não renomeado) para não quebrar o fluxo validado. _(ver changelog)_
- [x] **Multi-tenancy** — o tenant é resolvido pela API key (`auth.tenants: [{id, keys}]`; `api_keys` planas → `default`) e posto no contexto; records isolados por `(tenant, domain)` em todas as operações + ancoragem (batch ID namespaced por tenant). _(ver changelog)_ ⚠️ Isolamento de storage por **campo `tenant`** (não collection-por-tenant) e **mesmo channel** Fabric — o channel-por-org depende da rede de produção (item abaixo).
- [x] **Rede Fabric de produção (staging multi-org, 1 host)** — **3 peer orgs** (`Org1/2/3MSP`, peer + CouchDB cada), **Raft de 3 orderers** via **channel participation** (sem system-channel), **CAs separadas** (Fabric CA por org + CA do orderer, sem `cryptogen`) e política de endosso **`MAJORITY` 2/3**. Stack paralela e isolada da dev (`docker-compose-prod.yml`, rede `tcc_log_network_prod`, portas próprias, crypto em `organizations/`). `logchaincode` commitado (seq 1) e ancoragem cross-org validada — incl. **tolerância a falhas** (1 orderer down e 1 org down → ancora; 2 orgs down → rejeita). Artefatos em `hybrid-architecture/fabric-network/prod/` (`configtx.yaml`, composes, `scripts/`). _(ver changelog)_ ⚠️ **Pendente (plano de produção, pós-Fase 2):** Fase **F** (API ancorando contra a rede prod via discovery), Fase **H** (canal por tenant) e Fase **G** (hardening: segredos, TLS/DNS, multi-host). Ver [docs/plano-rede-fabric-producao.md](docs/plano-rede-fabric-producao.md).
- [x] **Webhook/callback ao ancorar** — pacote `internal/webhook`: ao ancorar um batch (logs ou records), dispara `POST` do evento `batch.anchored` (domain, batch_id, merkle_root, num_records, tx_id, anchored_at) para `webhook.url`, assinado com HMAC-SHA256 (`X-Webhook-Signature`) quando há `secret`; entrega assíncrona com retries. Opt-in via `webhook.enabled`. _(ver changelog)_
- [x] **SDK cliente em Go** — módulo `sdk/go` (pacote `anchor`, só stdlib): `Client` com retry automático (network/5xx) + métodos CRUD/batch/verify; **verificação de integridade local** (`ComputeHash`/`MerkleRoot`/`VerifyRecordsLocally`) que recalcula independente do servidor (no create, checa que o hash do servidor == hash local). _(ver changelog)_

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

### 2026-06-06 — Fase 2: rede Fabric de produção (staging multi-org) — **fecha a Fase 2 (7/7)**

Último item da Fase 2. Sai da rede mono-org de dev para uma rede **multi-organização
confiável**, em que ancorar exige endosso de orgs independentes. Conforme as decisões
travadas no [plano](docs/plano-rede-fabric-producao.md): **staging multi-org em 1 host**,
**Fabric CA por org** (sem `cryptogen`), endosso **`MAJORITY` 2/3**, **canal por tenant
dentro do MVP** (Fase H, pós-F).

- **Topologia (stack paralela e isolada da dev):** `docker-compose-ca.yml` (4 CAs:
  `ca.org1/2/3` + `ca.orderer`) + `docker-compose-prod.yml` (Raft de **3 orderers**, **3
  peer orgs** com CouchDB, CLI). Rede `tcc_log_network_prod`, portas deslocadas, crypto
  em `prod/organizations/`. A rede de dev (`tcc_log_network`, `make up`) **continua no ar
  validando em paralelo** — nada foi mutado.
- **Identidades reais via Fabric CA** (`scripts/registerEnroll.sh`): admin/peer/user por
  org, 3 orderers + admin do orderer org, MSP + TLS com NodeOUs. Domínio por org.
- **Bootstrap sem system-channel:** `configtx.yaml` (profile `ThreeOrgsChannel`, `MAJORITY
  Endorsement` = 2/3) renderizado direto no genesis do canal de aplicação; orderers com
  `BOOTSTRAPMETHOD=none` + `CHANNELPARTICIPATION_ENABLED=true`; canal `logchannel` criado
  via `osnadmin channel join` nos 3 orderers (`systemChannel: null`, confirmado).
- **Peers + chaincode:** os 3 peers joinaram `logchannel` (`join-peers.sh`); `logchaincode`
  instalado nas 3 orgs, aprovado por todas e **commitado** (v1.0 seq 1) satisfazendo a
  `MAJORITY` (`deploy-chaincode.sh`).
- **Validação E2E (CLI):** `smoke-test.sh` — `StoreMerkleRoot` endossado por **Org1+Org2
  (2 de 3)**, commit `VALID` nos dois peers (txid real), lido de volta pela **Org3**.
  `fault-tolerance.sh` — (1) Raft ancora com **1 orderer down**; (2) ancora com **1 org
  down** (2/3); (3) **rejeita** com **2 orgs down** (prova de que o endosso é real). Rede
  restaurada e íntegra após os testes.
- Novos scripts de operação/diagnóstico em `prod/scripts/`: `status.sh`,
  `orderer-status.sh`, `anchor.sh`, `fault-tolerance.sh` (além dos de deploy). Crypto e
  blocos ficam fora do git (`.gitignore`).
- ⚠️ **Restante do plano de produção (pós-Fase 2):** Fase **F** (API ancorando contra a
  rede prod via service discovery — _fecha o E2E do produto_), Fase **H** (um canal Fabric
  por tenant — isolamento no nível de ledger), Fase **G** (hardening: segredos como secret,
  TLS/DNS reais, multi-host, backup/DR). A Fase 2 do roadmap (a **rede** configurada e
  validada) está concluída; F/G/H são a ponte para produção real.

### 2026-06-05 — Fase 2: multi-tenancy (isolamento por API key)

Cada cliente (tenant) é resolvido pela API key e só enxerga os próprios records.

- **Auth tenant-aware:** `auth.tenants: [{id, keys}]` (chaves planas `api_keys` → tenant `default`); `AuthConfig.KeyToTenant()` monta o mapa key→tenant. O middleware `APIKeyAuth` resolve o tenant (constant-time) e o põe no contexto (`tenant`).
- **Records isolados por `(tenant, domain)`:** novo campo `Record.Tenant`; todas as operações (create/get/list/delete/batch/verify) filtram por tenant (do contexto, não da URL). Índice único `(tenant, domain, id)` + índice de cursor `(tenant, domain, created_at, id)`. Sem auth, tudo cai no tenant `default` (aditivo).
- **Ancoragem por tenant:** `ProcessRecordBatch`/`VerifyRecordBatch` recebem tenant; auto-batch itera scopes `(tenant, domain)` (`DistinctPendingRecordScopes`); batch ID namespaced `tenant-domain-uuid`; evento de webhook ganhou `tenant`.
- Testes: `database.TestRecordsCRUD` (isolamento tenant + domain), `middleware.TestAPIKeyAuth`/`TestMatchAPIKey` (resolução de tenant). **E2E ao vivo (2 tenants):** globex lendo record do acme → 404; cada um lista só os próprios records no mesmo domínio. `go build`/`vet`/`test ./...` limpos.
- ⚠️ **Escopo:** isolamento de storage por campo `tenant` (não collection-por-tenant) e **mesmo channel** Fabric. O channel-por-org depende da rede Fabric de produção (3 orgs) — único item restante da Fase 2.

### 2026-06-05 — Fase 2: SDK cliente Go (`anchor`) com verificação local

Entrega o "SDK embarcável" do vertical de Compliance.

- Novo módulo `sdk/go` (pacote `anchor`, **só stdlib**, sem dependências) — `github.com/RicardoMBregalda/tcc-log-management/sdk/go`.
- `Client` (`New` + `WithAPIKey`/`WithHTTPClient`/`WithMaxRetries`): `CreateRecord`, `GetRecord`, `BatchRecords`, `VerifyBatch`. **Retry automático** em erros de rede/5xx com backoff; 4xx vira `*APIError` sem retry; 409 do verify volta como resultado (IsValid=false).
- **Verificação de integridade local (trustless):** `(*Record).ComputeHash()` e `MerkleRoot`/`VerifyRecordsLocally` recalculam independentemente do servidor (mesmo algoritmo: `SHA-256(id|timestamp|source|payload canônico)`, payload canônico com chaves ordenadas). No `CreateRecord`, o id/timestamp são gerados no cliente e o hash do servidor é checado contra o hash local.
- Testes: unit (hash/merkle/tamper, retry 5xx, no-retry 4xx, mismatch de hash) + integração ao vivo (`-tags integration`): SDK criou record (hash do servidor == local), ancorou no Fabric (tx real) e `VerifyBatch` deu VALID. README em `sdk/go`. Build/test limpos.

### 2026-06-05 — Fase 2: webhook/callback ao ancorar batch

Notifica integrações externas quando um batch é ancorado na blockchain.

- Novo pacote `internal/webhook`: `Notifier` faz `POST` do evento `batch.anchored` (`domain`, `batch_id`, `merkle_root`, `num_records`, `tx_id`, `anchored_at`) para `webhook.url`. Assinatura **HMAC-SHA256** no header `X-Webhook-Signature` quando há `webhook.secret`. Entrega **assíncrona** (fire-and-forget) com retries — nunca bloqueia nem falha a batelada (o batch já está on-chain).
- Integrado no `BatchProcessor` (`SetNotifier` + `notifyAnchored`): dispara após ancorar tanto **logs** (`processBatch`) quanto **records** (`ProcessRecordBatch`).
- Config `webhook` (`enabled`, `url`, `secret`, `timeout`, `max_retries`) + env `WEBHOOK_*` + validação (enabled exige url). Opt-in.
- Testes: `internal/webhook` (payload + assinatura HMAC + retries + disabled, via httptest). **E2E ao vivo:** batch de records ancorado (tx real) → receiver local recebeu `batch.anchored` com assinatura válida. `go build`/`vet`/`test ./...` limpos.

### 2026-06-05 — Fase 2: ancoragem Merkle/Fabric generalizada para records

Fecha o ciclo tamper-evident no domínio genérico (era o follow-up declarado).

- `models.CalculateRecordMerkleRoot` recalcula o hash de cada record (do conteúdo) e constrói o root — recompute (não confia no hash guardado) = tamper-evident.
- `collections`: `FindRecordsWithoutBatch`/`FindRecordsByBatchID` (ordem determinística `created_at`+`id`), `UpdateRecordBatch`, `DistinctPendingRecordDomains`.
- `BatchProcessor.ProcessRecordBatch(domain, n)`: agrupa records pendentes do domínio, calcula o Merkle root, **ancora no Fabric** (`StoreMerkleRoot` via gateway, batch ID namespaced por domínio) e carimba os records. `VerifyRecordBatch` recalcula e compara. O auto-batch ticker passou a batelar records por domínio além de logs.
- API: `POST /api/v1/{domain}/records/batch` (força + ancora) e `POST /api/v1/{domain}/records/verify/{batchId}` (200 VALID / 409 CORRUPTED).
- **E2E ao vivo:** records no domínio `audit` → batch ancorado no Fabric (tx real, provado consultando o chaincode) → verify **VALID**; após adulterar um record no Mongo, verify **CORRUPTED (409)**. Teste unitário `CalculateRecordMerkleRoot` (determinismo + adulteração). `go build`/`vet`/`test ./...` limpos.

### 2026-06-05 — Fase 2: abstração Log → Record (genérico + hash configurável + /records)

Início da Fase 2 (generalização do domínio), de forma **aditiva** — `/logs` e o fluxo validado ficam intactos.

- **`models.Record`** (`internal/models/record.go`): registro genérico auditável — `domain`, `id`, `timestamp`, `source`, `payload` (JSON livre), `hash` + auditoria (`created_at`, `batch_id`, `merkle_root`, `batched_at`, `deleted_at`). Um `Log` é só um Record no domínio `logs`.
- **Hash configurável:** `SHA-256(id|timestamp|source|payload canônico)`; `hash_fields` opcional restringe quais chaves do payload entram no hash (guardado no record p/ reprodutibilidade). Canônico via `json.Marshal` (Go ordena chaves) → **independe da ordem** das chaves.
- **API `/api/v1/{domain}/records`** (`internal/handlers/records.go`): create / list (filtro `source` + paginação cursor/offset) / get / delete (soft). Protegida pela auth; reusa os helpers de cursor e soft delete.
- **Storage:** collection `records` (config `mongodb.records_collection`, env `MONGO_RECORDS_COLLECTION`) com índices `(domain,id)` único e `(domain,created_at,id)` p/ cursor.
- Testes: `models` (determinismo do hash por ordem de chave, `hash_fields`, validação) + `database.TestRecordsCRUD` (isolamento por domínio + soft delete, contra Mongo). Validado também rodando a API: `POST /api/v1/contracts/records` → 201 com hash; list ok. `go build`/`vet`/`test ./...` limpos.
- **Adiado (próximos incrementos):** ancoragem Merkle/Fabric dos records (hoje log-specific), WAL e cache p/ records, isolamento multi-tenant de storage/channel.

### 2026-06-04 — Validação E2E (rede viva) + 4 bugs corrigidos

Teste ponta a ponta pela API em container (gateway default) antes da Fase 2. O fluxo de produto funciona — log → Mongo → batch → Merkle root → **ancorado no Fabric via gateway** (provado consultando o chaincode) → `/merkle/batches` → **verificação de integridade VALID**. No caminho, o E2E pegou 4 bugs reais (todos corrigidos):

1. **Crash-loop do container (permissão de chave):** o container roda como não-root e lia a chave de identidade do gateway (`priv_sk`, gerada 0600/root) de um mount read-only → "permission denied" em loop. Fix: `1-generate-artifacts.sh` torna as chaves de dev legíveis (`chmod 0644`); em produção, provisionar a chave como secret com o owner do runtime.
2. **`/merkle/batches` 500:** `UpdateLogBatch`/`UpdateSyncStatusBatch` usavam `$currentDate` **dentro de `$set`**, gravando o objeto literal `{$currentDate:true}` em vez de uma data → o aggregate de batches falhava no decode. Fix: `batched_at` vira string RFC3339 (compatível com `BatchInfo.BatchedAt string` e `Log.BatchedAt FlexTime`); `synced_at` usa `$currentDate` no topo.
3. **"context canceled" nos workers:** `ProcessBatch` amarrava o job assíncrono ao contexto da requisição HTTP, cancelado ao responder 202. Fix: job usa `context.Background()`; o contexto do chamador só governa o enfileiramento.
4. **Falso "CORRUPTED" na integridade:** o Merkle root depende da ordem dos logs, e a criação (`FindLogsWithoutBatch`) e a verificação (`FindLogsByBatchID`) ordenavam só por `created_at` — com empates (logs em lote: 100 logs, 2 `created_at` distintos), a ordem divergia → root diferente. Fix: desempate determinístico por `id` (`created_at`+`id`) nas duas queries. Batch criado pós-fix verifica **VALID** (root recalculado == original).

`go build`/`vet`/`test ./...` limpos no Go 1.25.

### 2026-06-04 — Gateway promovido a transporte default

Com o E2E validado, o `gateway` vira o transporte padrão; `docker-exec` fica como fallback.

- `config`: default `fabric.transport: gateway` + caminhos de cert/identidade default apontando para o mount do docker-compose (`/fabric-crypto/...`). `config.yaml`, `.env.example` e README atualizados.
- `docker-compose`: `docker.sock` desmontado (comentado) — não é mais necessário no caminho default; o mount `crypto-config:/fabric-crypto:ro` sustenta o gateway. A API e os peers já compartilham a rede `tcc_log_network`, então o container resolve `peer0.org1.example.com:7051`.
- Validação: a API sobe com `transport=gateway` (log "Fabric client initialized") e `/health` reporta `fabric: healthy` (backend gateway construído: identidade + TLS carregados, gRPC client criado). Somado ao `TestGatewayE2E`, cobre construção + boot + transação real.
- _Nota de operação:_ para rodar **nativo** (fora do compose), use `FABRIC_TRANSPORT=docker-exec` ou sobrescreva os paths do gateway (os defaults são do container). O container `tcc-go-api` em execução ainda usa a imagem antiga — aplica-se com um rebuild (`make api`).

### 2026-06-04 — Fabric: upgrade para as versões mais recentes

Logo após a Fase B, subimos do stack mínimo para o mais novo (decisão de produto).

- **Go 1.21 → 1.25** (`go.mod` `go 1.25.0` + `Dockerfile` `golang:1.25-alpine`).
- **fabric-gateway v1.5.0 → v1.11.0** (latest), que arrastou grpc `v1.62 → v1.80` e protobuf `v1.34 → v1.36`. O `tidy` resolveu sem conflito.
- A API do cliente é estável no 1.x — **zero mudança** no `gatewayBackend`, exceto trocar `grpc.Dial` (deprecado no grpc novo) por `grpc.NewClient`.
- Revalidado: `go build`/`vet`/`test ./...` limpos no Go 1.25 **e** `TestGatewayE2E` rodou de novo contra a rede viva (novo txID real). 

### 2026-06-04 — Fabric via SDK, Fase B: backend Gateway (gRPC) — fecha a Fase 1

Último item da Fase 1. Substitui o `docker exec` por um cliente gRPC de verdade.

- **Bump de Go 1.18 → 1.21** (`go.mod` + `Dockerfile`), requisito do `fabric-gateway`. O `tidy` resolveu grpc/protobuf/x-libs sem conflito (e dispensa o pin antigo de `x/sys`).
- Novo `gatewayBackend` (`internal/fabric/gateway.go`) com `github.com/hyperledger/fabric-gateway` v1.5.0: conexão gRPC+TLS ao peer (root TLS do crypto-config), identidade X.509 (`Admin@org1`: signcert + chave do keystore), `NewProposal/Endorse/Submit` (com txID + commit status) e `EvaluateTransaction`.
- Factory `newBackend` agora constrói `gateway` quando `fabric.transport: gateway`; `disabledBackend` cobre `sync_enabled: false` (não exige conexão/cert).
- Config `gateway` (`msp_id`, `gateway_peer_endpoint`, `gateway_server_name_override`, `gateway_peer_tls_ca_file`, `identity_cert_file`, `identity_key_dir`) + env `FABRIC_*` + validação por transporte.
- **Bug corrigido:** os nomes de função agora batem com o chaincode (`StoreMerkleRoot`/`QueryMerkleBatch`), antes `storeMerkleBatch`/`getMerkleBatch` (inexistentes).
- `docker-compose`: monta `crypto-config` (`/fabric-crypto`, ro) e documenta que o `docker.sock` só serve ao transporte `docker-exec`. README/.env atualizados.
- Testes: `TestGatewayBackendConstruct` carrega a identidade/TLS **reais** do crypto-config e valida a construção do backend (grpc.Dial é lazy, não precisa da rede). `go build`/`vet`/`test ./...` limpos no Go 1.21.
- ✅ **E2E validado:** `TestGatewayE2E` (build tag `integration`) rodou contra a rede Fabric viva — `StoreMerkleRoot` (Submit, txID real `334372ea…`) + `QueryMerkleBatch` (Evaluate) com round-trip do `merkle_root`. O default segue `docker-exec` apenas porque os caminhos de cert do gateway são específicos do deploy; tecnicamente o gateway está pronto para virar default.

**Fase 1 concluída (8/8).** Próximo: Fase 2 (generalização do domínio) ou promover o gateway a default (fornecendo os caminhos de cert no deploy).

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
