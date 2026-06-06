# Plano — Rede Fabric de Produção (3 orgs, Raft 3 orderers, CAs separadas)

Documento interno de planejamento. Último item aberto da **Fase 2** do [ROADMAP](../ROADMAP.md)
(painel em `6 de 7`). Não executar antes de validar este plano.

## 1. Objetivo

Sair de uma rede **mono-organização de desenvolvimento** (1 peer org, 1 orderer, CA
central via `cryptogen`) para uma rede **multi-organização confiável**, em que a
ancoragem dos Merkle roots passa a exigir **endosso de organizações independentes** —
é isso que transforma o "tamper-evident" de uma promessa de uma única parte em uma
garantia distribuída.

### O que muda (atual → produção)

| Dimensão | Hoje (dev) | Produção (alvo) |
|---|---|---|
| Peer orgs | 1 (`Org1MSP`, 3 peers) | 3 (`Org1MSP`, `Org2MSP`, `Org3MSP`), peers próprios |
| Ordering | 1 orderer (Raft de 1 nó = sem tolerância a falha) | Raft de **3 orderers** (tolera 1 falha) |
| Identidades | `cryptogen` (autoridade única, só dev/test) | **Fabric CA por org** (enroll/register reais) + TLS CA |
| Genesis/canal | `system-channel` (método legado, `GENESISMETHOD=file`) | **Channel participation** (`osnadmin`, sem system-channel) |
| Endosso | `MAJORITY Endorsement` com 1 org = só Org1 | `MAJORITY` (2 de 3) entre orgs independentes |
| Domínios | `example.com` / `org1.example.com` (compartilhado) | um domínio por org (`org1.acme.io`, etc.) |
| Segredos | `priv_sk` afrouxado para 0644 (gambiarra dev) | chave como *secret* do runtime / PKCS#11/HSM |
| Topologia | 1 host, 1 docker-compose, rede `tcc_log_network` | orgs em hosts/VMs/k8s separados (ou staging 1-host) |
| API/gateway | conecta a `peer0.org1`, endosso de 1 org | discovery + coleta de endosso **cross-org** |

## 2. Decisão-chave de topologia

Há dois níveis de "produção", e eles têm custos muito diferentes:

- **(S) Staging multi-org em 1 host** — três orgs reais (MSP, CA, peers, política de
  endosso de 2/3), mas todos os contêineres no mesmo Docker host. Valida *toda* a
  lógica de multi-org (endosso, discovery, lifecycle de chaincode com 3 aprovações,
  canais por tenant) localmente, do mesmo jeito que a rede atual é validada.
- **(P) Produção real multi-host** — cada org em sua própria infraestrutura
  (VM/k8s), CAs operadas por cada parte, rede sobreposta/TLS com DNS real, HSM.
  Aqui o gargalo é **infra e governança**, não código.

> **Recomendação:** entregar **(S) primeiro**, com todos os artefatos **parametrizados
> por org** (domínio, portas, caminhos de cert) de modo que mover para **(P)** seja
> trocar endpoints e onde cada contêiner roda — não reescrever a topologia.

### Estratégia de reversibilidade (não quebrar o que valida hoje)

A rede de dev atual valida todos os E2Es do produto. A rede nova deve nascer como uma
**stack separada e paralela**, sem mutar a existente:

- Novo compose: `hybrid-architecture/fabric-network/docker-compose.prod.yml`
- Nova rede Docker: `tcc_log_network_prod` (não a `tcc_log_network`)
- Novo diretório de crypto: `crypto-config-prod/` (não tocar em `crypto-config/`)
- Portas deslocadas (ex.: orderers 7050/8050/9050; peers por org em faixas distintas)
- Novos profiles em `configtx.yaml` (`ThreeOrgsGenesis`, `ThreeOrgsChannel`) — **adicionar**, não substituir os atuais

Assim, `make up` (dev) continua funcionando; a rede de produção sobe por um alvo novo
(`make up-prod`) e pode ser derrubada sem afetar a validação corrente.

## 3. Fases

Cada fase é incremental, testável isoladamente e não derruba a anterior.

### Fase A — Identidades por org via Fabric CA  *(DECIDIDO: CA real desde o início)*
- **Sem `cryptogen`.** Cada org sobe seu próprio **`fabric-ca-server`** (CA de
  identidade + TLS CA) e as identidades são provisionadas com
  `fabric-ca-client register`/`enroll`: admin da org, cada peer, cada orderer e o
  usuário da API. Domínio próprio por org.
- **Entregáveis:**
  - serviços `ca.org{1,2,3}` no `docker-compose.prod.yml` (3 CAs);
  - `scripts/ca/registerEnroll.sh` parametrizado por org (gera a estrutura MSP/TLS em
    `crypto-config-prod/`), espelhando o padrão `test-network` oficial do Fabric;
  - `scripts/ca/ccp-generate.sh` opcional (connection profile por org para o gateway).
- **Risco:** alto. Substitui todo o bootstrap de `cryptogen`; é a fundação das demais
  fases. Validar `enroll` de uma org ponta a ponta antes de replicar para as três.

### Fase B — Ordering Raft de 3 nós + channel participation
- 3 orderers (`orderer0/1/2`), cada um no conjunto de **consenters** do `configtx`.
- Migrar do `system-channel` legado para **channel participation API**: gerar o bloco
  de genesis do *canal de aplicação* com `configtxgen`, subir orderers com
  `ORDERER_CHANNELPARTICIPATION_ENABLED=true` e `ORDERER_ADMIN_*`, e criar o canal via
  `osnadmin channel join` em cada orderer.
- **Entregáveis:** serviços de orderer no `docker-compose.prod.yml`, bloco
  `genesis-block` do canal, ajuste do `scripts/1-generate-artifacts.sh` (sem
  system-channel), novo `scripts/2-init-network.sh` usando `osnadmin`.
- **Risco:** alto. É a mudança estrutural mais sensível (consenso + bootstrap).

### Fase C — 3 peer orgs + gossip/anchor peers
- Peers por org, cada um com seu CouchDB, anchor peer por org no `configtx`,
  `CORE_PEER_GOSSIP_EXTERNALENDPOINT` correto por org.
- **Entregáveis:** serviços de peer/couchdb por org no compose; anchor peers no `configtx`.
- **Risco:** médio.

### Fase D — `configtx` com 3 orgs e política de endosso
- Novos profiles com as 3 orgs no Application/Consortium.
- **Política de endosso da aplicação:** `MAJORITY Endorsement` agora significa **2 de 3
  orgs** — decisão de produto (alternativas: `AND(Org1,Org2,Org3)` = todas, ou
  `OutOf(2, ...)`). Recomendo `MAJORITY` (2/3): tolera 1 org indisponível e ainda exige
  independência.
- **Entregáveis:** `configtx.yaml` (profiles `ThreeOrgs*`).
- **Risco:** baixo (config declarativa), mas define a semântica de confiança.

### Fase E — Lifecycle do chaincode com 3 aprovações
- `peer lifecycle chaincode install` nas 3 orgs, `approveformyorg` por org, e `commit`
  com `--peerAddresses` das 3 (satisfazendo a `MAJORITY`).
- Atenção à **endorsement policy do chaincode** no commit (`--signature-policy` ou herdar
  a do canal). Deve casar com a Fase D.
- **Entregáveis:** novo `scripts/2-init-network.sh` (loop por org) ou
  `scripts/deploy-chaincode-prod.sh`.
- **Risco:** médio (vários pontos onde o MSP/endpoint errado falha silenciosamente).

### Fase F — API/gateway coletando endosso cross-org
- O `fabric-gateway` (já default) coleta endosso de várias orgs **via service
  discovery**, desde que o peer-gateway da org da API conheça os endpoints e CAs das
  outras orgs. Verificar:
  - discovery habilitado e o peer-gateway com gossip para as demais orgs;
  - a identidade da API (cliente de uma org, ex. Org1) tem permissão de Writers no canal;
  - timeouts de endosso suficientes para round-trip entre 3 orgs.
- Pode ser necessário expor no `config.yaml` a lista de orgs/endpoints de endosso ou
  confiar 100% no discovery. Validar `SubmitTransaction` com a política 2/3 ativa.
- **Entregáveis:** ajustes em `api/pkg/config` (se preciso parametrizar orgs de endosso),
  `api/internal/fabric/*` (gateway), `config.yaml`.
- **Risco:** médio. É onde "rede pronta" vira "produto ancora de verdade".

### Fase G — Segredos, TLS e hardening
- Chaves privadas como *secret* do runtime (não 0644); idealmente PKCS#11/HSM para a
  identidade da API e dos orderers.
- TLS com SANs/DNS reais por org; rotação/renovação de certificados (a CA da Fase A2).
- Logging de produção, limites de recurso, `operations`/Prometheus por nó (já há
  `:9443`), backup do ledger por org, plano de DR.
- **Entregáveis:** `docker-compose.prod.yml` (secrets), docs de operação.
- **Risco:** baixo a médio, mas obrigatório para "produção" de verdade.

### Fase H — Canais por tenant  *(DECIDIDO: dentro do MVP)*
- Com 3 orgs reais, criar **um canal por tenant** (ex.: `acme-channel`), elevando o
  isolamento de "campo `tenant` no mesmo canal" para **isolamento no nível de ledger** —
  fecha o caveat registrado na entrega de multi-tenancy.
- O chaincode é instanciado em cada canal de tenant; o `config.yaml`/`fabric` mapeia
  **tenant → canal**, e o `batch_processor` ancora no canal do tenant resolvido pela API.
- **Entregáveis:** mapeamento tenant→canal em `api/pkg/config` + uso em
  `api/internal/fabric` e `batch_processor`; `scripts/create-tenant-channel.sh`
  (parametrizado) via `osnadmin`/`peer channel`.
- **Risco:** médio. Faz parte do corte de produção; entra **depois** de F (gateway
  multi-org provado em 1 canal) para isolar a complexidade.

## 4. Ordem recomendada e esforço  *(ajustada às decisões)*

1. **A** (Fabric CA por org) + **B** (Raft 3 + osnadmin) + **C** (3 peer orgs) +
   **D** (configtx 3 orgs, `MAJORITY` 2/3) + **E** (lifecycle com 3 aprovações) —
   núcleo multi-org com identidades reais, validável localmente. *Risco concentrado em A e B.*
2. **F** — API ancorando com endosso 2/3 via discovery. *Fecha o E2E do produto.*
3. **H** — um canal Fabric por tenant. *Isolamento no nível de ledger (no MVP).*
4. **G** — hardening (segredos como secret, TLS/DNS, backup/DR, observabilidade).

## 5. Validação E2E (critérios de aceite)

- [x] `osnadmin channel list` mostra o canal nos 3 orderers (`logchannel` em
      orderer0/1/2, `systemChannel: null` = channel participation). Raft tolera
      derrubar 1 orderer sem parar de ancorar. ✅ _(validado 2026-06-06)_
- [x] `peer lifecycle chaincode querycommitted` confirma o chaincode (`logchaincode`
      v1.0 seq 1) com a política 2/3. ✅
- [x] Ancoragem **com endosso de ≥2 orgs** retorna `tx_id` real, commit `VALID` nos
      peers endossantes (`StoreMerkleRoot` endossado por Org1+Org2, lido de volta pela
      Org3). ✅ _Nota:_ validado via **CLI** (`smoke-test.sh`); a ancoragem pela **API**
      contra esta rede é a **Fase F** (abaixo, pendente).
- [x] Derrubar **1 org** e ainda ancorar (2/3 tolera); derrubar **2 orgs** → ancoragem
      **rejeitada** (prova de que o endosso é real). ✅ _(`fault-tolerance.sh`)_
- [ ] Verificação de integridade `VALID`/`CORRUPTED` (tamper) **pela API** contra esta
      rede — depende da Fase F (API apontada para a rede prod).
- [x] A rede de dev (`make up`) continua intacta e validando em paralelo
      (`tcc_log_network` + containers `*.example.com` no ar). ✅

> **Onde parou (2026-06-06):** o núcleo multi-org (Fases **A–E** da seção 4) está
> **deployado e validado** — 4 CAs, Raft de 3 orderers via channel participation, 3 peer
> orgs com CouchDB, canal `logchannel`, `logchaincode` commitado com `MAJORITY` 2/3, e
> ancoragem cross-org provada (incl. tolerância a falhas). **Restam:** Fase **F** (API
> ancorando contra a rede prod via discovery), Fase **H** (canal por tenant) e Fase **G**
> (hardening). Scripts de operação/validação em `prod/scripts/`:
> `registerEnroll.sh`, `join-peers.sh`, `deploy-chaincode.sh`, `smoke-test.sh`,
> `status.sh`, `orderer-status.sh`, `anchor.sh`, `fault-tolerance.sh`.

## 6. Riscos e mitigação (resumo)

| Risco | Mitigação |
|---|---|
| Quebrar a rede de dev que valida tudo | Stack paralela (`*.prod.yml`, rede/portas/crypto próprios); nunca mutar a atual |
| Bootstrap `osnadmin`/Raft 3 nós (mais sensível) | Isolar na Fase B; testar orderers antes de qualquer peer |
| Endosso cross-org não fechar no gateway | Validar discovery na Fase F com a política já ativa; teste de derrubar 1 org |
| `cryptogen` não é produção | A1 para staging, A2 (Fabric CA) antes de chamar de produção |
| Ambiente local não comporta 3 orgs (CPU/mem) | Cada peer/orderer com `cpus`/`mem_limit`; medir; se faltar recurso, staging em menos réplicas |

## 7. Decisões travadas (2026-06-05)

1. **Escopo:** **Staging multi-org em 1 host (S)** primeiro, parametrizado para depois ir a multi-host.
2. **Identidades:** **Fabric CA por org desde o início** (sem `cryptogen`).
3. **Política de endosso:** **`MAJORITY` (2 de 3)**.
4. **Canais por tenant (Fase H):** **dentro do MVP** (depois de F).

## 8. Pré-requisito de viabilidade (1 host)

Esta topologia sobe ~**16+ contêineres** (3 CAs + 3 orderers + 3 peers + 3 CouchDB +
CLI + a stack do produto: API/Mongo/Redis). Antes de começar a Fase A, **medir
CPU/memória disponíveis** e dimensionar `cpus`/`mem_limit` por nó. Se o host não
comportar, reduzir réplicas no staging (ex.: 1 peer por org em vez de vários) sem mudar
a lógica multi-org.
