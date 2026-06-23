---
title: Kubernetes (Helm)
description: Deploy AeternisLog to Kubernetes with the Helm chart — migration Job, WAL persistence, replica gating, and least-privilege identity.
sidebar:
  order: 3
---

The Helm chart (`deploy/helm/aeternislog`) deploys the API with production-shaped
defaults.

## Install

```bash
helm install aeternislog deploy/helm/aeternislog \
  --set image.tag=<your-tag> \
  --set fabric.peerEndpoint=peer0.org1.example.com:7051
```

## What the chart does

- **Migration Job:** a `pre-install,pre-upgrade` hook runs the migrator **before**
  the API rolls out. The API Deployment only *asserts* the schema version — it
  never mutates. See [migrations](/aeternis-log/operations/migrations/).
- **WAL persistence:** the WAL is mounted on a PVC so a record durably written by a
  pod survives reschedules.
- **Replica gating:** `replicaCount > 1` is rejected unless you set
  `allowMultiReplica=true`, because the file WAL is per-pod (RWO PVC). Run a single
  replica, or arrange per-pod persistence (a StatefulSet) first.
- **Least-privilege identity:** the API runs as a non-root user (uid 1000) and
  mounts only its own Fabric identity bundle (cert + key `0400` + peer TLS CA),
  injected via a Secret.
- **Secrets, not ConfigMaps:** API keys and the webhook secret are rendered into a
  Kubernetes Secret and injected as env vars (keys may be `sha256:<hash>`).

## Datastores

The chart ships **demo single-pod** MongoDB and Redis for evaluation. For
production, run external managed instances:

```yaml
mongodb:
  enabled: false   # point mongodb.url at a managed replica set
redis:
  enabled: false   # use a managed Redis (optional cache/limiter)
```

## Fabric network

The Fabric network is external to the cluster. The chart configures the gateway
endpoint, channel, chaincode, MSP, and per-tenant channels/identities. See the
multi-org production-staging scripts under
`hybrid-architecture/fabric-network/prod/`.

## Custom domain & TLS

Enable the `ingress` block (host, className, TLS) to expose the API behind your
own hostname.
