# Runbook de Operação e Hardening — Rede Fabric de Produção

Fase **G** do [plano de rede de produção](plano-rede-fabric-producao.md). Documenta o
hardening da rede multi-org: **o que já está aplicado no staging (1 host)** e **o que
falta para produção real (multi-host)** — onde o gargalo é infra/governança, não código.

> **Legenda:** ✅ aplicado no staging · 🟡 parcial/staging-only · ⬜ requer infra real (multi-host).

---

## 1. Segredos e identidades

### Estado atual (staging) ✅
- **API non-root + identidade de menor privilégio.** A API roda como uid `1000`
  (`appuser`) e monta **apenas** seu bundle de identidade
  (`prod/api-identity/org1`: admin cert + chave `0400` + TLS CA do peer), gerado por
  [`build-api-identity.sh`](../hybrid-architecture/fabric-network/prod/scripts/build-api-identity.sh) —
  **não** a árvore `organizations/` inteira (que contém as chaves de todas as orgs e dos
  orderers). Reduz tanto o privilégio do processo quanto a superfície do mount.
- Crypto e bundles **fora do git** (`.gitignore`: `organizations/`, `api-identity/`,
  `*.block`, `*.tar.gz`).
- **API keys e webhook secret em Secret, não em ConfigMap (F05).** O chart Helm
  renderiza `auth.apiKeys`, as chaves de cada tenant e `webhook.secret` num
  **Kubernetes Secret** (`<release>-secrets`), injetado na API via `envFrom`
  (`AUTH_API_KEYS` / `AUTH_TENANTS` / `WEBHOOK_SECRET`) — nunca no ConfigMap.
  Uma chave pode ser **plaintext** ou um **hash `sha256:<hex>`**
  (`printf '%s' "$KEY" | sha256sum`), de modo que a chave crua não precisa ficar
  em repouso. Alterar o Secret rola os pods (`checksum/secret`).

### Rotação de API keys (sem downtime)
1. Gere a nova chave; adicione-a ao tenant **junto** da antiga (`keys: [old, new]`).
2. `helm upgrade` → o `checksum/secret` rola os pods; ambas as chaves ficam válidas.
3. Migre os clientes para a nova chave.
4. Remova a antiga (`keys: [new]`) e `helm upgrade` novamente.

### Para produção real ⬜
- **Chaves nunca em disco do app.** Provisionar a chave de assinatura via:
  - **PKCS#11 / HSM** (SoftHSM em teste; HSM real em prod) — o `fabric-gateway` e os
    peers/orderers suportam BCCSP PKCS#11. A identidade da API passa a assinar pelo HSM.
  - **Secret manager** — Kubernetes Secrets (montados como `tmpfs`), HashiCorp Vault
    (com agent injector) ou Docker Swarm secrets. Em swarm/k8s o `uid/gid/mode` do
    secret é respeitado (no `docker compose` single-host não é — por isso aqui usamos o
    bundle com dono `1000`).
- **Senhas reais** nas CAs (`-b admin:adminpw`) e CouchDB (`admin:password`) → mover para
  secrets; rotacionar.
- Princípio: a identidade da API deveria ser um **usuário `client` dedicado** (não o
  `Admin` da org). Trocar `Admin@org1` por um `apiuser@org1` com permissão só de Writer.

---

## 2. TLS, DNS e rotação de certificados

### Estado atual (staging) 🟡
- TLS habilitado ponta a ponta (peers, orderers, CAs, gateway). Certs emitidos pela
  Fabric CA com SANs (`--csr.hosts`) incluindo o hostname do nó e `localhost`.
- Domínios são `*.example.com` resolvidos pela rede Docker (`aeternislog_network_prod`).

### Para produção real ⬜
- **DNS real por org** (`peer0.org1.acme.io`, etc.) com SANs corretos; cada org opera sua
  própria CA/infra. Reemitir certs com os hosts reais.
- **Rotação/renovação:** a Fabric CA permite `fabric-ca-client reenroll` antes da
  expiração. Definir validade (`--csr.expiry` / perfis da CA) e um job de renovação.
  Certs TLS dos orderers entram no `configtx` como consenters — renovar exige
  **channel config update** (não só trocar o arquivo).
- `mutual TLS` (clientAuth) no gateway/peers se a política exigir.

---

## 3. Observabilidade (operations / Prometheus)

### Endpoints já expostos ✅
| Componente | Endpoint operations/metrics |
|---|---|
| peers (org1/2/3) | `:9443/metrics` (host `7161`/`7261`/`7361`) |
| orderers (0/1/2) | `:9443/metrics` (operations) |
| API | `:9090/metrics` (host `9091` no stack prod) — Prometheus |

### Para produção real ⬜
- Scrape via Prometheus + dashboards Grafana padronizados (Fase 4 do roadmap).
- Alertas: batch pendente > X min, discrepância de Merkle root, WAL acima de threshold,
  Raft sem líder, peer fora do canal (ver Fase 4 do roadmap).

---

## 4. Limites de recurso (sizing) ✅ (no compose)

Definidos por nó (`mem_limit`/`cpus`) nos composes — aplicam no próximo `up`/recreate
(dados persistem nos volumes nomeados):

| Nó | mem_limit | cpus |
|---|---|---|
| peer0.org{1,2,3} | 1g | 1.0 |
| couchdb.org{1,2,3} | 1g | 1.0 |
| orderer{0,1,2} | 512m | 0.5 |
| ca.* / cli | 256m–512m | 0.5 |
| API / Mongo / Redis (prod) | 512m / 1g / 512m | 1.0 / 1.0 / 0.5 |

Uso medido em repouso é bem abaixo disso (peers ~70 MiB, orderers ~25 MiB); os limites
são **tetos** com folga. Medir sob carga real antes de fixar em prod:
`docker stats --no-stream`.

---

## 5. Backup e Disaster Recovery

### Backup ✅
[`backup-ledger.sh`](../hybrid-architecture/fabric-network/prod/scripts/backup-ledger.sh)
gera tar.gz por nó dos ledgers **autoritativos** (3 orderers + 3 peers) em
`prod/backups/<timestamp>/`. O **CouchDB é derivado** e reconstruído do ledger no boot do
peer — não precisa backup.

```bash
bash prod/scripts/backup-ledger.sh        # backup live (crash-consistent)
```
Para backup estritamente consistente, parar o nó antes (`docker stop`), tarar, religar;
com Raft, um orderer por vez.

### Restore / DR (procedimento)
1. Subir a infra (CAs, orderers, peers) com os **mesmos volumes/crypto** ou restaurar o
   crypto de um cofre.
2. Para cada nó, restaurar o ledger no volume parado:
   ```bash
   docker run --rm --volumes-from <container> -v <backupdir>:/backup alpine \
     sh -c 'rm -rf /var/hyperledger/production/* && \
            tar xzf /backup/<node>.tar.gz -C /var/hyperledger/production'
   ```
   (orderers: caminho `/var/hyperledger/production/orderer`.)
3. Subir os peers; o CouchDB reconstrói o state a partir do ledger.
4. Validar: [`status.sh`](../hybrid-architecture/fabric-network/prod/scripts/status.sh),
   [`orderer-status.sh`](../hybrid-architecture/fabric-network/prod/scripts/orderer-status.sh)
   e [`smoke-test.sh`](../hybrid-architecture/fabric-network/prod/scripts/smoke-test.sh).

### Para produção real ⬜
- Backup **offsite** (S3/objeto) com retenção e criptografia; testar restore
  periodicamente (DR drill).
- Como o ledger é **replicado entre 3 orgs/orderers**, a perda de 1 nó é recuperável pelo
  próprio cluster (re-sync via Raft/gossip) — backup cobre o desastre correlacionado
  (perda do host inteiro).

---

## 6. Tolerância a falhas (já validada) ✅

[`fault-tolerance.sh`](../hybrid-architecture/fabric-network/prod/scripts/fault-tolerance.sh)
prova: Raft tolera 1 orderer down; ancora com 1 org down (2/3); **rejeita** com 2 orgs
down. Re-rodar após mudanças estruturais.

---

## 7. O que falta para "produção real" (multi-host, P)

Itens cujo gargalo é **infra e governança**, não código (cada org em sua própria
infra/VM/k8s):

- ⬜ Cada org operando sua **própria CA/peers/orderer** em hosts separados, com rede
  sobreposta/TLS e **DNS real**.
- ⬜ Chaves em **HSM/PKCS#11** ou secret manager (seção 1).
- ⬜ Governança de **channel config updates** assinada por MAJORITY de admins (rotação de
  cert de orderer, adição de org, mudança de política).
- ⬜ Backup **offsite** + DR drills agendados; observabilidade central (Fase 4).

A topologia do staging já é **parametrizada por org** (domínio, portas, caminhos de
cert), então a ida para multi-host é trocar endpoints e onde cada contêiner roda — não
reescrever a rede.
