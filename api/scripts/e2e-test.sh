#!/usr/bin/env bash
# Black-box end-to-end QA for the API: exercises the full HTTP surface against a
# running instance backed by a live Fabric network, with assertions. Dependency-free
# (curl + grep/sed). Exits non-zero if any check fails.
#
# Config via env:
#   BASE          API base URL                 (default http://localhost:5002)
#   METRICS       Prometheus metrics URL       (default http://localhost:9091/metrics)
#   KEY           API key for tenant 1         (default staging-default-key; "" = no auth)
#   EXPECT_CHANNEL  channel tenant 1 anchors to (default logchannel)
#   KEY2          API key for tenant 2         (default staging-acme-key; "" = skip isolation)
#   MONGO         mongo container for tamper    (default tcc-mongodb-prod; "" = skip tamper)
#
#   bash e2e-test.sh
set -uo pipefail

BASE="${BASE:-http://localhost:5002}"
METRICS="${METRICS:-http://localhost:9091/metrics}"
KEY="${KEY-staging-default-key}"
EXPECT_CHANNEL="${EXPECT_CHANNEL:-logchannel}"
KEY2="${KEY2-staging-acme-key}"
MONGO="${MONGO-tcc-mongodb-prod}"

BODY=$(mktemp)
PASS=0; FAIL=0
trap 'rm -f "$BODY"' EXIT

c_green=$'\033[32m'; c_red=$'\033[31m'; c_off=$'\033[0m'
pass() { PASS=$((PASS+1)); echo "  ${c_green}PASS${c_off} $1"; }
fail() { FAIL=$((FAIL+1)); echo "  ${c_red}FAIL${c_off} $1"; echo "        body: $(head -c 300 "$BODY")"; }
section() { echo; echo "### $1"; }

# _curl <key> <method> <path> [data]  -> sets CODE, RESP
_curl() {
  local key="$1" method="$2" path="$3" data="${4-}"
  local args=(-s -o "$BODY" -w '%{http_code}' -X "$method" "$BASE$path")
  [ -n "$key" ] && args+=(-H "X-API-Key: $key")
  [ -n "$data" ] && args+=(-H 'Content-Type: application/json' -d "$data")
  CODE=$(curl "${args[@]}"); RESP=$(cat "$BODY")
}
req()  { _curl "$KEY"  "$1" "$2" "${3-}"; }   # tenant 1
req2() { _curl "$KEY2" "$1" "$2" "${3-}"; }   # tenant 2

ck_code() { if [ "$CODE" = "$1" ]; then pass "$2 (HTTP $CODE)"; else fail "$2 (want $1, got $CODE)"; fi; }
ck_has()  { if echo "$RESP" | grep -q "$1"; then pass "$2"; else fail "$2 (missing: $1)"; fi; }
# extract first JSON string value for a key
jget() { echo "$RESP" | sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" | head -1; }

echo "=== E2E QA against $BASE (auth=$([ -n "$KEY" ] && echo on || echo off), channel=$EXPECT_CHANNEL) ==="

# ---------------------------------------------------------------- infra
section "Infra / health"
req GET /health;  ck_code 200 "GET /health"; ck_has '"fabric":"healthy"' "fabric healthy"
ck_has '"mongodb":"healthy"' "mongodb healthy"; ck_has '"redis":"healthy"' "redis healthy"
req GET /;        ck_code 200 "GET / (root)"
mbody=$(curl -s -w $'\n%{http_code}' "$METRICS"); mcode=$(echo "$mbody" | tail -1)
if [ "$mcode" = 200 ]; then pass "GET metrics (HTTP 200)"; else fail "metrics endpoint (got $mcode)"; fi
if echo "$mbody" | grep -q '^go_goroutines'; then pass "metrics expose Prometheus data (go_goroutines)"; else fail "metrics body not Prometheus"; fi
if echo "$mbody" | grep -q '^http_requests_total'; then pass "metrics expose http_requests_total"; else fail "http_requests_total missing"; fi

# ---------------------------------------------------------------- auth
if [ -n "$KEY" ]; then
  section "Auth"
  _curl "" POST /api/v1/qa/records '{"source":"x","payload":{}}'; ck_code 401 "no key -> 401"
  _curl "wrong-key" POST /api/v1/qa/records '{"source":"x","payload":{}}'; ck_code 401 "wrong key -> 401"
fi

# ---------------------------------------------------------------- records CRUD
section "Records CRUD (domain qa)"
req POST /api/v1/qa/records '{"source":"qa-src","payload":{"event":"login","user":"u1"}}'
ck_code 201 "create record"; RID=$(jget id); RHASH=$(jget hash)
if echo "$RHASH" | grep -qE '^[0-9a-f]{64}$'; then pass "hash is 64-hex"; else fail "hash format ($RHASH)"; fi
req GET "/api/v1/qa/records/$RID"; ck_code 200 "get record"; ck_has "\"hash\":\"$RHASH\"" "stored hash matches create"
ck_has '"user":"u1"' "payload preserved"
req GET /api/v1/qa/records; ck_code 200 "list records"; ck_has "$RID" "list contains created"
req GET "/api/v1/qa/records?source=qa-src"; ck_code 200 "list filtered by source"

section "Soft delete + duplicate id (audit immutability)"
DELID="qa-del-$(date +%s%N)"
req POST /api/v1/qa/records "{\"id\":\"$DELID\",\"source\":\"qa-src\",\"payload\":{\"x\":1}}"; ck_code 201 "create with explicit id"
req POST /api/v1/qa/records "{\"id\":\"$DELID\",\"source\":\"qa-src\",\"payload\":{\"x\":2}}"; ck_code 409 "duplicate id -> 409 (not 500)"
req DELETE "/api/v1/qa/records/$DELID"; ck_code 200 "soft delete"
req GET "/api/v1/qa/records/$DELID"; ck_code 404 "deleted hidden from get"
req POST /api/v1/qa/records "{\"id\":\"$DELID\",\"source\":\"qa-src\",\"payload\":{\"x\":3}}"; ck_code 409 "resurrect soft-deleted id -> 409"

# ---------------------------------------------------------------- cursor pagination
section "Cursor pagination"
for i in 1 2 3; do req POST /api/v1/pg/records "{\"source\":\"pg\",\"payload\":{\"n\":$i}}" >/dev/null; done
req GET "/api/v1/pg/records?limit=2"; ck_code 200 "page 1 (limit 2)"
p1=$(echo "$RESP" | grep -o '"id":"[^"]*"' | sort)
NC=$(jget next_cursor)
if [ -n "$NC" ]; then pass "next_cursor present"; else fail "next_cursor present"; fi
req GET "/api/v1/pg/records?limit=2&cursor=$NC"; ck_code 200 "page 2 via cursor"
p2=$(echo "$RESP" | grep -o '"id":"[^"]*"' | sort)
overlap=$(comm -12 <(echo "$p1") <(echo "$p2") | grep -c '"id"')
if [ "$overlap" = 0 ]; then pass "no overlap between pages"; else fail "pages overlap ($overlap ids)"; fi

# ---------------------------------------------------------------- batch + anchor + verify + tamper
section "Batch / anchor / verify (Fabric)"
req POST /api/v1/audit/records '{"source":"qa","payload":{"e":"a"}}' >/dev/null
req POST /api/v1/audit/records '{"source":"qa","payload":{"e":"b"}}' >/dev/null
req POST /api/v1/audit/records/batch '{"batch_size":100}'
ck_code 200 "force batch"; ck_has '"anchored":true' "anchored true"
BID=$(jget batch_id); TX=$(jget tx_id); CH=$(jget channel)
if [ -n "$TX" ]; then pass "real tx_id ($TX)"; else fail "tx_id present"; fi
if [ "$CH" = "$EXPECT_CHANNEL" ]; then pass "anchored to expected channel ($CH)"; else fail "channel: want $EXPECT_CHANNEL got $CH"; fi
req POST "/api/v1/audit/records/verify/$BID"; ck_code 200 "verify"; ck_has '"integrity":"VALID"' "integrity VALID"
if curl -s "$METRICS" | grep -q '^batches_anchored_total'; then pass "metrics: batches_anchored_total recorded"; else fail "batches_anchored_total missing after anchor"; fi

if [ -n "$MONGO" ]; then
  section "Tamper detection"
  docker exec "$MONGO" mongosh logdb --quiet --eval "db.records.updateOne({batch_id:'$BID'},{\$set:{'payload._qa_tamper':'1'}})" >/dev/null 2>&1
  req POST "/api/v1/audit/records/verify/$BID"; ck_code 409 "tampered verify -> 409"; ck_has '"integrity":"CORRUPTED"' "integrity CORRUPTED"
fi

# ---------------------------------------------------------------- multi-tenant isolation
if [ -n "$KEY2" ] && [ -n "$KEY" ]; then
  section "Multi-tenant isolation"
  req POST /api/v1/iso/records '{"source":"t1","payload":{"secret":"t1-only"}}'; ck_code 201 "tenant1 creates"
  IID=$(jget id)
  req GET "/api/v1/iso/records/$IID"; ck_code 200 "tenant1 reads own"
  req2 GET "/api/v1/iso/records/$IID"; ck_code 404 "tenant2 cannot read tenant1 record"
fi

# ---------------------------------------------------------------- logs domain (legacy) + merkle
section "Logs domain + Merkle"
req POST /logs '{"source":"qa-logs","level":"INFO","message":"hello","metadata":{"k":"v"}}'
if [ "$CODE" = 201 ] || [ "$CODE" = 200 ]; then pass "POST /logs (HTTP $CODE)"; else fail "POST /logs (got $CODE)"; fi
req GET /logs; ck_code 200 "GET /logs"
req POST /merkle/force-batch '{"batch_size":100}'
if [ "$CODE" = 200 ] || [ "$CODE" = 202 ]; then pass "force log batch (HTTP $CODE)"; else fail "force log batch (got $CODE)"; fi
req GET /merkle/batches; ck_code 200 "GET /merkle/batches"

# ---------------------------------------------------------------- stats + wal
section "Stats / WAL"
req GET /stats;       ck_code 200 "GET /stats"
req GET /stats/logs;  ck_code 200 "GET /stats/logs"
req GET /stats/sync;  ck_code 200 "GET /stats/sync"
req GET /wal/stats;   ck_code 200 "GET /wal/stats"
req GET /wal/health;  ck_code 200 "GET /wal/health"

# ---------------------------------------------------------------- summary
echo
echo "=================================================="
echo "  PASS=$PASS  FAIL=$FAIL"
echo "=================================================="
[ "$FAIL" = 0 ]
