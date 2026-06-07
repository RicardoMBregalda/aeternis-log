# Observability — Prometheus alerts + Grafana dashboard

The API exposes Prometheus metrics on its metrics port (`metrics.port`, default `9090`)
at `/metrics`: HTTP request rate/latency, Go/process runtime, and product counters
(`batches_anchored_total`, `records_anchored_total`, `integrity_verifications_total`).

## Scrape the API

```yaml
# prometheus.yml
scrape_configs:
  - job_name: anchor-api
    static_configs:
      - targets: ["anchor-api:9090"]   # host:metricsPort
rule_files:
  - prometheus-alerts.yml
```

In Kubernetes (Helm chart) the API pod carries `prometheus.io/scrape` annotations, so a
Prometheus configured for pod annotation discovery picks it up automatically.

## Alerts

[`prometheus-alerts.yml`](prometheus-alerts.yml) ships these rules:

| Alert | Meaning | Severity |
|---|---|---|
| `AnchorApiDown` | API unscrapable for 2m | critical |
| `IntegrityViolationDetected` | a batch verified as **CORRUPTED** (Merkle discrepancy) | critical |
| `AnchoringStalled` | records written but none anchored for 15m | warning |
| `HighErrorRate` | >5% 5xx over 10m | warning |
| `HighWriteLatencyP99` | P99 record-write latency > 500ms (SLA) for 10m | warning |

Validate with `promtool check rules prometheus-alerts.yml`.

## Grafana dashboard

Import [`grafana-dashboard.json`](grafana-dashboard.json) (Dashboards → Import) and pick
your Prometheus data source. Panels: request rate by status, write latency (p50/p99),
5xx ratio, batches anchored, integrity verifications (VALID/CORRUPTED), memory, goroutines.

## SLA — write latency P99 < 500ms

The `http_request_duration_seconds` histogram lets you measure and enforce the
record-write SLA. Query the P99 over the create-record route:

```promql
histogram_quantile(0.99,
  sum by (le) (rate(http_request_duration_seconds_bucket{route="/api/v1/:domain/records", method="POST"}[5m]))
)
```

The `HighWriteLatencyP99` alert fires when this exceeds `0.5` (500ms) for 10 minutes;
the "Write latency (POST records)" dashboard panel tracks p50/p99 continuously. WAL
durability (`wal.backend`) is the main latency lever — the file backend with `fsync`
trades a little latency for zero data loss; benchmark with your durability setting before
committing to the SLA.

