#!/usr/bin/env bash
# Deploy logchaincode to the 3-org channel using the Fabric chaincode lifecycle.
# Install on every org's peer, approve for all 3 orgs, then commit satisfying the
# channel's MAJORITY endorsement (2 of 3). Runs inside the CLI container.
set -e

CC_NAME=logchaincode
CC_VERSION=1.0
CC_SEQ=1
CC_LABEL=logchaincode_1
CC_PATH=/opt/gopath/src/github.com/chaincode
CHANNEL=logchannel
ORDERER=orderer0.example.com:7050
ORDERER_CA=/opt/org/ordererOrganizations/example.com/orderers/orderer0.example.com/tls/ca.crt
ORG_BASE=/opt/org/peerOrganizations
PKG=/opt/gopath/src/github.com/hyperledger/fabric/peer/logchaincode.tar.gz

setorg() {
  local N="$1"
  export CORE_PEER_TLS_ENABLED=true
  export CORE_PEER_LOCALMSPID="Org${N}MSP"
  export CORE_PEER_ADDRESS="peer0.org${N}.example.com:7051"
  export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org${N}.example.com/users/Admin@org${N}.example.com/msp"
  export CORE_PEER_TLS_ROOTCERT_FILE="$ORG_BASE/org${N}.example.com/peers/peer0.org${N}.example.com/tls/ca.crt"
}

peerca() { echo "$ORG_BASE/org${1}.example.com/peers/peer0.org${1}.example.com/tls/ca.crt"; }

echo "=== package ==="
setorg 1
peer lifecycle chaincode package "$PKG" --path "$CC_PATH" --lang golang --label "$CC_LABEL"

echo "=== install on all orgs (builds chaincode image; first run is slow) ==="
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
[ -n "$PACKAGE_ID" ] || { echo "ERROR: empty package id"; exit 1; }

echo "=== approve for each org ==="
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
if peer lifecycle chaincode querycommitted --channelID "$CHANNEL" --name "$CC_NAME" 2>/dev/null | grep -q "Version: $CC_VERSION"; then
  echo "already committed"
else
  peer lifecycle chaincode commit -o "$ORDERER" --tls --cafile "$ORDERER_CA" \
    --channelID "$CHANNEL" --name "$CC_NAME" --version "$CC_VERSION" --sequence "$CC_SEQ" \
    --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles "$(peerca 1)" \
    --peerAddresses peer0.org2.example.com:7051 --tlsRootCertFiles "$(peerca 2)" \
    --peerAddresses peer0.org3.example.com:7051 --tlsRootCertFiles "$(peerca 3)"
fi

echo "=== querycommitted ==="
peer lifecycle chaincode querycommitted --channelID "$CHANNEL" --name "$CC_NAME"
