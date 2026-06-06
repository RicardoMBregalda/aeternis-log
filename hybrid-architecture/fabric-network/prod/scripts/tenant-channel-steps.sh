#!/usr/bin/env bash
# Inside cli.prod: join a tenant channel on the 3 orderers (osnadmin) and the 3
# peers, then approve+commit logchaincode on it (MAJORITY 2/3). logchaincode is
# already installed on the peers (per-peer package shared across channels), so
# this skips install. Idempotent: skips steps already done.
#
# Called by create-tenant-channel.sh. Usage (inside cli.prod): tenant-channel-steps.sh <channel-id>
set -e

CHANNEL="${1:?channel id required}"
BLOCK="/opt/channel-artifacts/${CHANNEL}.block"
OORG=/opt/org/ordererOrganizations/example.com
ORG_BASE=/opt/org/peerOrganizations
ORDERER=orderer0.example.com:7050
ORDERER_CA="$OORG/orderers/orderer0.example.com/tls/ca.crt"
CC_NAME=logchaincode
CC_VERSION=1.0
CC_SEQ=1
CC_LABEL=logchaincode_1

# orderer0's TLS leaf doubles as the osnadmin mutual-TLS client cert (same CA).
OADM_CERT="$OORG/orderers/orderer0.example.com/tls/server.crt"
OADM_KEY="$OORG/orderers/orderer0.example.com/tls/server.key"

echo "=== osnadmin: join 3 orderers to $CHANNEL ==="
for i in 0 1 2; do
  if osnadmin channel list -o "orderer${i}.example.com:7053" --ca-file "$ORDERER_CA" \
       --client-cert "$OADM_CERT" --client-key "$OADM_KEY" 2>/dev/null | grep -q "\"name\": \"$CHANNEL\""; then
    echo "    orderer${i}: already joined"
  else
    echo "    orderer${i}: joining"
    osnadmin channel join --channelID "$CHANNEL" --config-block "$BLOCK" \
      -o "orderer${i}.example.com:7053" --ca-file "$ORDERER_CA" \
      --client-cert "$OADM_CERT" --client-key "$OADM_KEY"
    sleep 1
  fi
done
sleep 2

setorg() {
  local N="$1"
  export CORE_PEER_TLS_ENABLED=true
  export CORE_PEER_LOCALMSPID="Org${N}MSP"
  export CORE_PEER_ADDRESS="peer0.org${N}.example.com:7051"
  export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org${N}.example.com/users/Admin@org${N}.example.com/msp"
  export CORE_PEER_TLS_ROOTCERT_FILE="$ORG_BASE/org${N}.example.com/peers/peer0.org${N}.example.com/tls/ca.crt"
}
peerca() { echo "$ORG_BASE/org${1}.example.com/peers/peer0.org${1}.example.com/tls/ca.crt"; }

echo "=== peer join 3 orgs to $CHANNEL ==="
for N in 1 2 3; do
  setorg "$N"
  if peer channel list 2>/dev/null | grep -qx "$CHANNEL"; then
    echo "    Org${N}: already joined"
  else
    echo "    Org${N}: joining"
    peer channel join -b "$BLOCK"
    sleep 2
  fi
done

echo "=== resolve installed package id ==="
setorg 1
PACKAGE_ID=$(peer lifecycle chaincode queryinstalled 2>/dev/null | sed -n "/$CC_LABEL/{s/^Package ID: //;s/, Label:.*//;p}" | head -1)
echo "    PACKAGE_ID=$PACKAGE_ID"
[ -n "$PACKAGE_ID" ] || { echo "ERROR: logchaincode not installed; run deploy-chaincode.sh first"; exit 1; }

echo "=== approve logchaincode for each org on $CHANNEL ==="
for N in 1 2 3; do
  setorg "$N"
  if peer lifecycle chaincode checkcommitreadiness --channelID "$CHANNEL" --name "$CC_NAME" \
       --version "$CC_VERSION" --sequence "$CC_SEQ" --output json 2>/dev/null | grep -q "\"Org${N}MSP\": true"; then
    echo "    Org${N}: already approved"
  else
    echo "    Org${N}: approving"
    peer lifecycle chaincode approveformyorg -o "$ORDERER" --tls --cafile "$ORDERER_CA" \
      --channelID "$CHANNEL" --name "$CC_NAME" --version "$CC_VERSION" \
      --package-id "$PACKAGE_ID" --sequence "$CC_SEQ"
    sleep 2
  fi
done

echo "=== commit (MAJORITY 2/3) on $CHANNEL ==="
setorg 1
if peer lifecycle chaincode querycommitted --channelID "$CHANNEL" --name "$CC_NAME" 2>/dev/null | grep -q "Version: $CC_VERSION"; then
  echo "    already committed"
else
  peer lifecycle chaincode commit -o "$ORDERER" --tls --cafile "$ORDERER_CA" \
    --channelID "$CHANNEL" --name "$CC_NAME" --version "$CC_VERSION" --sequence "$CC_SEQ" \
    --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles "$(peerca 1)" \
    --peerAddresses peer0.org2.example.com:7051 --tlsRootCertFiles "$(peerca 2)" \
    --peerAddresses peer0.org3.example.com:7051 --tlsRootCertFiles "$(peerca 3)"
fi

echo "=== querycommitted on $CHANNEL ==="
peer lifecycle chaincode querycommitted --channelID "$CHANNEL" --name "$CC_NAME"
