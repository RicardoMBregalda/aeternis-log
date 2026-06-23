# AeternisLog - Integrity Dashboard

A dependency-free web dashboard for the Tamper-Evident Data Anchoring API. It
talks only to the public + records surface (`/health`, `/api/v1/{domain}/...`,
`/public/anchors/...`):

- **Health** - live Mongo / Redis / Fabric / batch-processor chips with honest
  states: `healthy` and `running` read as up, `disabled` / `not configured` as a
  neutral off, and only real `unhealthy:` checks turn red. The overall LED tracks
  the API's `healthy` / `degraded` status.
- **Overview** - KPI tiles (records, anchored batches, anchored records, services
  up) plus two charts derived from the domain audit report: an **integrity
  coverage** gauge (records covered by an on-chain proof) and an **anchoring
  activity** bar chart (records anchored over time, from each batch's timestamp).
- **Records (log viewer)** - every record is an expandable row. The collapsed row
  shows time, source, an auto-detected level and message (when the payload has
  them), anchor status and hash; expanding it reveals the full, syntax-highlighted
  JSON payload and the integrity proof (id, hash, batch, Merkle root, Fabric tx),
  with one-click verify of its batch. Filter loaded records client-side, by level,
  or by source on the server. Create a record inline and **anchor the pending
  pool** into a Merkle batch.
- **Anchor status** - a distribution bar of the records currently loaded across
  the lifecycle (anchored / pending / unbatched / failed).
- **Anchored batches** - every batch with its Merkle root, record-count bar and
  tx id, plus one-click **verify** (recompute server-side and compare to the
  on-chain anchor) and a download link for the audit report PDF.
- **Public verification** - the unauthenticated path an external auditor uses:
  prove a batch is anchored and optionally check that a Merkle root you hold
  matches the anchored one, without an API key and leaking no tenant metadata.

It is a single static `index.html` (vanilla JS, no build step, no third-party
libraries; the charts and icons are hand-rolled inline SVG). Connection settings
(API URL, API key, domain) are stored in the browser's `localStorage`, and an
optional **auto** toggle refreshes health, overview and batches every 10s.

It is built for accessibility: keyboard-operable expandable rows, visible focus
rings, `prefers-reduced-motion` support, and WCAG AA contrast in its dark theme.

## Run

The dashboard is part of the standard stack. `make up` (from the repo root)
builds and starts it together with the API, the datastores and the Fabric
network. It is served by nginx (defined in `api/docker-compose.yml`) at:

    http://localhost:8088

To start, restart or tail just the dashboard:

```bash
make dashboard        # nginx on :8088
make dashboard-logs   # follow its logs
```

Or, for a quick look without Docker, serve the folder directly:

```bash
python3 -m http.server 8088 --directory examples/dashboard
```

Enter the API base URL (e.g. `http://localhost:5001`), the records **domain**,
and - if the API has auth enabled - an API key. After `make up` the defaults
(`http://localhost:5001`, domain `audit`) already point at the local API. The
API enables permissive CORS, so the dashboard can run from any origin.

> For production, serve the dashboard behind the same origin as the API (or a
> trusted host) and scope CORS accordingly.
