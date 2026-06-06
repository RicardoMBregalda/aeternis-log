# Anchor — Integrity Dashboard

A minimal, dependency-free web dashboard for the Tamper-Evident Data Anchoring API:

- **Health** — live API / Fabric / Mongo status.
- **Sync status** — anchored vs pending counts (`/stats/sync`).
- **Log batches** — anchored Merkle batches with one-click integrity **verify**
  (recompute and compare against the anchored root).
- **Verify a record batch** — check any domain batch by id.

It is a single static `index.html` (vanilla JS, no build step). Connection settings
(API URL, API key, domain) are stored in the browser's `localStorage`.

## Run

Open `index.html` directly, or serve the folder:

```bash
python3 -m http.server 8088 --directory dashboard
# then open http://localhost:8088
```

Enter the API base URL (e.g. `http://localhost:5001`) and, if the API has auth
enabled, an API key. The API enables permissive CORS, so the dashboard can run from
any origin.

> For production, serve the dashboard behind the same origin as the API (or a
> trusted host) and scope CORS accordingly.
