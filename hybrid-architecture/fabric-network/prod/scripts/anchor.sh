#!/usr/bin/env bash
# Parametric anchor: invoke StoreMerkleRoot for a given batch id, collecting
# endorsements from a given set of peer orgs. Submits to a chosen orderer.
# Used by the fault-tolerance acceptance tests. Runs inside the CLI container.
#
# Usage: anchor.sh <batch_id> "<endorser_orgs>" [orderer_host]
#   anchor.sh batch-x "1 2"              # endorse with Org1+Org2, submit to orderer0
#   anchor.sh batch-y "1" orderer1.example.com:7050
set -e

BATCH_ID="${1:?batch id required}"
ENDORSERS="${2:?endorser orgs required, e.g. \"1 2\"}"
ORDERER="${3:-orderer0.example.com:7050}"

CHANNEL=logchannel
CC=logchaincode
ORG_BASE=/opt/org/peerOrganizations
OORG=/opt/org/ordererOrganizations/example.com
ORDERER_CA="$OORG/orderers/orderer0.example.com/tls/ca.crt"
peerca() { echo "$ORG_BASE/org${1}.example.com/peers/peer0.org${1}.example.com/tls/ca.crt"; }

# Sign as Org1 admin (any member of an endorsing org works as the submitter).
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID=Org1MSP
export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org1.example.com/users/Admin@org1.example.com/msp"
export CORE_PEER_TLS_ROOTCERT_FILE="$(peerca 1)"

PEER_ARGS=()
for N in $ENDORSERS; do
  PEER_ARGS+=( --peerAddresses "peer0.org${N}.example.com:7051" --tlsRootCertFiles "$(peerca "$N")" )
done

peer chaincode invoke -o "$ORDERER" --tls --cafile "$ORDERER_CA" -C "$CHANNEL" -n "$CC" \
  "${PEER_ARGS[@]}" \
  -c "{\"function\":\"StoreMerkleRoot\",\"Args\":[\"${BATCH_ID}\",\"root-${BATCH_ID}\",\"2026-06-06T00:00:00Z\",\"1\",\"[\\\"id1\\\"]\"]}" \
  --waitForEvent
