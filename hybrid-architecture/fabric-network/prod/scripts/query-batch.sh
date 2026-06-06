#!/usr/bin/env bash
# Read-only: query a Merkle batch on-chain from a given org's peer. Proves a batch
# anchored by the API (via Org1) is on the shared multi-org ledger and readable by
# another org. Runs inside the CLI container.
#
# Usage: query-batch.sh <batch_id> [org_num]   (org defaults to 3)
set -e
BATCH_ID="${1:?batch id required}"
N="${2:-3}"
ORG_BASE=/opt/org/peerOrganizations

export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org${N}MSP"
export CORE_PEER_ADDRESS="peer0.org${N}.example.com:7051"
export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org${N}.example.com/users/Admin@org${N}.example.com/msp"
export CORE_PEER_TLS_ROOTCERT_FILE="$ORG_BASE/org${N}.example.com/peers/peer0.org${N}.example.com/tls/ca.crt"

echo ">>> QueryMerkleBatch($BATCH_ID) from Org${N}"
peer chaincode query -C logchannel -n logchaincode \
  -c "{\"function\":\"QueryMerkleBatch\",\"Args\":[\"${BATCH_ID}\"]}"
