#!/usr/bin/env bash
# Upgrade logchaincode on the 3-org channel via the Fabric chaincode lifecycle.
#
# An upgrade is the same lifecycle as the initial deploy, at a NEW version and a
# NEW (incremented) sequence: package -> install on every org -> approve for all
# 3 orgs at the new sequence -> commit satisfying MAJORITY (2 of 3). The world
# state is preserved; only the chaincode definition/binary changes. Runs inside
# the CLI container.
#
# Usage:
#   upgrade-chaincode.sh <new-version> [new-sequence]
#   - <new-version>  required, e.g. 1.1, 2.0
#   - [new-sequence] optional; defaults to (current committed sequence + 1)
#
# Example (deploying the F14 tenant-scoped chaincode):
#   upgrade-chaincode.sh 2.0
set -euo pipefail

CC_NAME=logchaincode
CC_PATH=/opt/gopath/src/github.com/chaincode
CHANNEL=logchannel
ORDERER=orderer0.example.com:7050
ORDERER_CA=/opt/org/ordererOrganizations/example.com/orderers/orderer0.example.com/tls/ca.crt
ORG_BASE=/opt/org/peerOrganizations

CC_VERSION="${1:-}"
if [ -z "$CC_VERSION" ]; then
  echo "ERROR: new version required. Usage: upgrade-chaincode.sh <new-version> [new-sequence]" >&2
  exit 2
fi
CC_LABEL="${CC_NAME}_$(echo "$CC_VERSION" | tr '.' '_')"
PKG="/opt/gopath/src/github.com/hyperledger/fabric/peer/${CC_LABEL}.tar.gz"

setorg() {
  local N="$1"
  export CORE_PEER_TLS_ENABLED=true
  export CORE_PEER_LOCALMSPID="Org${N}MSP"
  export CORE_PEER_ADDRESS="peer0.org${N}.example.com:7051"
  export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org${N}.example.com/users/Admin@org${N}.example.com/msp"
  export CORE_PEER_TLS_ROOTCERT_FILE="$ORG_BASE/org${N}.example.com/peers/peer0.org${N}.example.com/tls/ca.crt"
}
peerca() { echo "$ORG_BASE/org${1}.example.com/peers/peer0.org${1}.example.com/tls/ca.crt"; }

# Derive the next sequence from what is currently committed, unless given.
setorg 1
QC=$(peer lifecycle chaincode querycommitted --channelID "$CHANNEL" --name "$CC_NAME" 2>/dev/null) || true
if [ -n "$QC" ]; then
  # The chaincode is already committed: parse its sequence. A committed chaincode
  # whose sequence we cannot read is an error, not a silent "start at 1".
  CURRENT_SEQ=$(printf '%s\n' "$QC" | sed -n 's/.*Sequence: \([0-9]\+\).*/\1/p' | head -1)
  if [ -z "$CURRENT_SEQ" ]; then
    echo "ERROR: '$CC_NAME' is committed but its sequence could not be parsed from:" >&2
    printf '%s\n' "$QC" >&2
    echo "Pass the new sequence explicitly: upgrade-chaincode.sh <version> <sequence>" >&2
    exit 2
  fi
else
  CURRENT_SEQ=0   # not committed yet — this is the first deploy
fi
CC_SEQ="${2:-$((CURRENT_SEQ + 1))}"
if [ "$CC_SEQ" -le "$CURRENT_SEQ" ]; then
  echo "ERROR: new sequence $CC_SEQ must be greater than the committed sequence $CURRENT_SEQ" >&2
  exit 2
fi
echo "Upgrading $CC_NAME: version=$CC_VERSION sequence=$CC_SEQ (was sequence $CURRENT_SEQ)"

echo "=== package ==="
peer lifecycle chaincode package "$PKG" --path "$CC_PATH" --lang golang --label "$CC_LABEL"

echo "=== install on all orgs (rebuilds chaincode image; first run is slow) ==="
for N in 1 2 3; do
  setorg "$N"
  if peer lifecycle chaincode queryinstalled 2>/dev/null | grep -q "$CC_LABEL"; then
    echo "Org${N}: already installed"
  else
    echo "Org${N}: installing..."
    peer lifecycle chaincode install "$PKG"
  fi
done

echo "=== resolve package id ==="
setorg 1
PACKAGE_ID=$(peer lifecycle chaincode queryinstalled 2>/dev/null | sed -n "/$CC_LABEL/{s/^Package ID: //;s/, Label:.*//;p}" | head -1)
echo "PACKAGE_ID=$PACKAGE_ID"
[ -n "$PACKAGE_ID" ] || { echo "ERROR: empty package id" >&2; exit 1; }

echo "=== approve for each org at sequence $CC_SEQ ==="
for N in 1 2 3; do
  setorg "$N"
  if peer lifecycle chaincode checkcommitreadiness --channelID "$CHANNEL" --name "$CC_NAME" \
       --version "$CC_VERSION" --sequence "$CC_SEQ" --output json 2>/dev/null | grep -q "\"Org${N}MSP\": true"; then
    echo "Org${N}: already approved"
  else
    echo "Org${N}: approving..."
    peer lifecycle chaincode approveformyorg -o "$ORDERER" --tls --cafile "$ORDERER_CA" \
      --channelID "$CHANNEL" --name "$CC_NAME" --version "$CC_VERSION" \
      --package-id "$PACKAGE_ID" --sequence "$CC_SEQ"
    sleep 2
  fi
done

echo "=== commit readiness ==="
setorg 1
peer lifecycle chaincode checkcommitreadiness --channelID "$CHANNEL" --name "$CC_NAME" \
  --version "$CC_VERSION" --sequence "$CC_SEQ" --output json

echo "=== commit (MAJORITY of orgs) ==="
peer lifecycle chaincode commit -o "$ORDERER" --tls --cafile "$ORDERER_CA" \
  --channelID "$CHANNEL" --name "$CC_NAME" --version "$CC_VERSION" --sequence "$CC_SEQ" \
  --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles "$(peerca 1)" \
  --peerAddresses peer0.org2.example.com:7051 --tlsRootCertFiles "$(peerca 2)" \
  --peerAddresses peer0.org3.example.com:7051 --tlsRootCertFiles "$(peerca 3)"

echo "=== querycommitted ==="
peer lifecycle chaincode querycommitted --channelID "$CHANNEL" --name "$CC_NAME"
echo "Upgrade to version=$CC_VERSION sequence=$CC_SEQ complete."
