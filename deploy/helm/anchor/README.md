# anchor Helm chart

Deploys the Tamper-Evident Data Anchoring API with MongoDB and Redis on Kubernetes.
The Hyperledger Fabric network is **external** to the cluster — the API's gRPC gateway
dials `fabric.peerEndpoint` and collects endorsement via service discovery — so the
chart only needs the client signing identity, provided through a Kubernetes Secret.

The API runs **non-root** (uid 1000) and mounts the identity read-only with the key file
isolated in its own directory and mode `0440` (group-readable by the runtime uid),
matching the production hardening in [docs/runbook-operacao-prod.md](../../../docs/runbook-operacao-prod.md).

## Install

```bash
helm install anchor deploy/helm/anchor \
  --set image.repository=ghcr.io/you/anchor-api --set image.tag=1.0 \
  --set-file fabric.identity.tlsCaCert=peer-tls-ca.pem \
  --set-file fabric.identity.signCert=user-cert.pem \
  --set-file fabric.identity.signKey=user-key.pem \
  --set fabric.peerEndpoint=peer0.org1.example.com:7051 \
  --set fabric.serverNameOverride=peer0.org1.example.com
```

For production, manage the identity with an external secret store and reference it:

```bash
helm install anchor deploy/helm/anchor --set fabric.identity.existingSecret=anchor-fabric-identity
```

The referenced Secret must have keys `tls-ca.pem`, `sign-cert.pem`, `sign-key.pem`.

## Key values

| Key | Default | Notes |
|---|---|---|
| `image.repository` / `image.tag` | `tcc-go-api` / `latest` | API image |
| `replicaCount` | `1` | with `wal.backend=redis` you can scale out |
| `auth.enabled`, `auth.apiKeys`, `auth.tenants` | off | per-tenant API keys |
| `fabric.channel` / `fabric.chaincode` | `logchannel` / `logchaincode` | |
| `fabric.tenantChannels` | `{}` | tenant → channel (ledger-level isolation) |
| `fabric.peerEndpoint` / `fabric.serverNameOverride` | dev defaults | external Fabric peer |
| `fabric.identity.*` | empty | signing identity (Secret) |
| `mongodb.enabled` / `redis.enabled` | `true` | disable to use managed services |
| `metrics.enabled` | `true` | Prometheus scrape annotations on the pod |
| `ingress.enabled` | `false` | expose the API |

## Validate

```bash
helm lint deploy/helm/anchor
helm template anchor deploy/helm/anchor -f my-values.yaml
```
