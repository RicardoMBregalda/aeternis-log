# Roadmap: Do TCC ao Produto

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
| Sem autenticação/autorização na API | Qualquer requisição é aceita | `handlers/logs.go` |
| WAL é arquivo local | Impede escala horizontal da API | `wal/wal.go` |
| `fmt.Printf("DEBUG ...")` em handlers | Não pode ir para produção | `handlers/logs.go:59` |
| Container names hardcoded (`peer0.org1.example.com`) | Frágil para qualquer deploy real | `fabric/client.go:50` |
| `DeleteLog` é no-op silencioso | Retorna 200 sem deletar nada | `handlers/logs.go:338` |
| Rede Fabric de dev (1 org, 1 peer, 1 orderer) | Sem tolerância a falhas na blockchain | `fabric-network/` |
| Paginação por offset | Não escala para datasets grandes | `handlers/logs.go:204` |

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

### Fase 1 — Estabilização Técnica (2-3 meses)

**Objetivo:** Tornar o código executável em ambiente real, não só em dev.

- [ ] Substituir `docker exec` no `FabricClient` pela integração real com `fabric-sdk-go` (o SDK já está no `go.mod` mas não é usado)
- [x] Remover todos os `fmt.Printf("DEBUG ...")` do código de produção — substituído por **structured logging com `zerolog`** (escolhido em vez de `slog` por compatibilidade com Go 1.18 e por ser zero-alocação no hot-path). Novo pacote `internal/logger`; logs em JSON com `request_id`, `service`, `caller`. _(ver changelog)_
- [ ] Implementar autenticação: JWT para API keys de clientes, middleware em `internal/middleware/`
- [ ] Mover WAL para solução distribuída (Redis Streams ou Kafka) para permitir múltiplas instâncias da API
- [ ] Substituir paginação por offset por cursor (campo `created_at` + ID como cursor)
- [ ] Implementar `DeleteLog` real com soft delete (campo `deleted_at`, não remove da blockchain)
- [ ] Variáveis de ambiente para todas as configurações hardcoded (container names, paths)
- [ ] Adicionar rate limiting no middleware (já existe o pacote, falta implementação)

### Fase 2 — Generalização do Domínio (2-3 meses)

**Objetivo:** Remover o acoplamento ao domínio de "logs" para suportar qualquer tipo de registro auditável.

- [ ] Abstrair `Log` para `Record` com schema flexível — campos obrigatórios: `id`, `timestamp`, `source`, `payload` (qualquer JSON), `hash`
- [ ] Tornar o `CalculateHash` configurável por schema: o cliente define quais campos entram no hash
- [ ] Renomear rotas de `/api/v1/logs` para `/api/v1/records` com namespace de domínio (`/api/v1/{domain}/records`)
- [ ] Multi-tenancy: cada cliente/organização tem seu próprio channel no Fabric e collection no MongoDB
- [ ] Configurar rede Fabric de produção: mínimo 3 orgs, consenso Raft com 3 orderers, CAs separadas
- [ ] Webhook/callback quando um batch é ancorado na blockchain (para integrações externas)
- [ ] SDK cliente em Go com suporte a retry automático e verificação de integridade local

### Fase 3 — Experiência do Desenvolvedor (2-3 meses)

**Objetivo:** Reduzir o tempo de integração de semanas para horas.

- [ ] SDK em Python (maior mercado de ciência de dados e compliance scripts)
- [ ] CLI para verificação offline: dado um arquivo CSV de registros, verifica se o Merkle Root bate com o da blockchain
- [ ] Dashboard web mínimo: listar batches, ver status de sincronização, verificar integridade
- [ ] Helm chart para deploy no Kubernetes (API + MongoDB + Fabric peer como sidecar)
- [ ] Documentação com guias por vertical: "Como usar para auditoria LGPD", "Como usar para supply chain"
- [ ] Sandbox público com rede Fabric de teste para desenvolvedores avaliarem

### Fase 4 — Observabilidade e SLAs (1-2 meses)

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
