#!/usr/bin/env bash
# Join the single peer of each org (Org1/Org2/Org3) to logchannel, then set the
# anchor peer. Runs inside the CLI container (paths under /opt). Uses each org's
# Admin identity. Idempotent: skips a peer already in the channel.
set -e

ORG_BASE=/opt/org/peerOrganizations
BLOCK=/opt/channel-artifacts/logchannel.block
ORDERER_CA=/opt/org/ordererOrganizations/example.com/orderers/orderer0.example.com/tls/ca.crt
CHANNEL=logchannel

setorg() {
  local N="$1"
  export CORE_PEER_TLS_ENABLED=true
  export CORE_PEER_LOCALMSPID="Org${N}MSP"
  export CORE_PEER_ADDRESS="peer0.org${N}.example.com:7051"
  export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org${N}.example.com/users/Admin@org${N}.example.com/msp"
  export CORE_PEER_TLS_ROOTCERT_FILE="$ORG_BASE/org${N}.example.com/peers/peer0.org${N}.example.com/tls/ca.crt"
}

for N in 1 2 3; do
  setorg "$N"
  echo ">>> Org${N}: joining peer0.org${N}.example.com"
  if peer channel list 2>/dev/null | grep -qx "$CHANNEL"; then
    echo "    already joined"
  else
    peer channel join -b "$BLOCK"
    echo "    joined"
    sleep 2
  fi
done

echo
echo "=== membership check ==="
for N in 1 2 3; do
  setorg "$N"
  echo "Org${N} channels: $(peer channel list 2>/dev/null | tail -n +2 | tr '\n' ' ')"
done
