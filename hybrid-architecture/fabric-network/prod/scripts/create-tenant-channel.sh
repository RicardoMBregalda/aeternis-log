#!/usr/bin/env bash
# Create a per-tenant application channel on the prod network and deploy
# logchaincode on it — the Phase H step that raises tenant isolation from a
# "tenant" field on the shared channel to a dedicated ledger per tenant.
#
# Reuses the ThreeOrgsChannel profile (same 3 orgs / MAJORITY 2/3), only the
# channel id changes. RUNS ON THE HOST (configtxgen + bin tools); the on-network
# steps run inside cli.prod via tenant-channel-steps.sh. Idempotent.
#
#   bash create-tenant-channel.sh acme-channel
set -euo pipefail

CHANNEL="${1:?usage: create-tenant-channel.sh <channel-id>}"
FN_DIR="/root/aeternis-log/hybrid-architecture/fabric-network"
PROD="$FN_DIR/prod"
BLOCK="$PROD/channel-artifacts/${CHANNEL}.block"

export FABRIC_CFG_PATH="$PROD"          # where configtx.yaml lives
export PATH="$FN_DIR/bin:$PATH"

echo "=== 1) configtxgen: genesis block for $CHANNEL (profile ThreeOrgsChannel) ==="
configtxgen -profile ThreeOrgsChannel -channelID "$CHANNEL" -outputBlock "$BLOCK"
echo "    wrote $BLOCK"

echo "=== 2) join orderers + peers, deploy chaincode (inside cli.prod) ==="
docker exec cli.prod bash /opt/scripts/tenant-channel-steps.sh "$CHANNEL"

echo
echo "Tenant channel '$CHANNEL' ready (logchaincode committed, MAJORITY 2/3)."
