---
title: Observability
description: Health checks, Prometheus metrics, and structured logs for operating AeternisLog.
sidebar:
  order: 8
---

## Health

```bash
GET /health
```

```json
{ "status": "healthy", "services": { "mongodb": "healthy", "redis": "healthy",
  "fabric": "healthy", "batch_processor": "running" } }
```

Use it for liveness/readiness probes. The Helm chart wires `/health` into both
probes. A degraded dependency (e.g. Redis down) is reported per-service so you can
alert precisely.

## Metrics (Prometheus)

```yaml
metrics:
  enabled: true
  port: 9090
  path: "/metrics"
```

The API exposes request, batching, and anchoring metrics at `:9090/metrics`. The
Helm pod carries Prometheus scrape annotations when `metrics.podAnnotations` is on.
Suggested signals to alert on:

- anchor failures / reconciler retries climbing,
- batch backlog (pending records not being anchored),
- request error rate and latency,
- Fabric health flapping.

## Logs

Structured logging (zerolog), configurable:

```yaml
logging:
  level: "info"     # debug, info, warn, error
  format: "json"    # json or console
  output: "stdout"
  enable_caller: true
```

Each request carries a request id (also in the `X-Request-ID` response header) so
you can correlate a client call across the logs. Key lifecycle events —
`database schema verified`, WAL recovery counts, batch anchored, reconcile actions
— are logged explicitly.

## Smoke test

`make smoke` is a fast black-box end-to-end check (create → anchor → verify →
tamper → reconcile) you can run after a deploy or on a schedule.
