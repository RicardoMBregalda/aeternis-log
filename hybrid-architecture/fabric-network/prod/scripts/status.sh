#!/usr/bin/env bash
# Read-only status probe for the production-staging network. Reports, per org:
# channel membership and committed chaincode. Runs inside the CLI container.
set +e

ORG_BASE=/opt/org/peerOrganizations
CHANNEL=logchannel
CC_NAME=logchaincode

setorg() {
  local N="$1"
  export CORE_PEER_TLS_ENABLED=true
  export CORE_PEER_LOCALMSPID="Org${N}MSP"
  export CORE_PEER_ADDRESS="peer0.org${N}.example.com:7051"
  export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org${N}.example.com/users/Admin@org${N}.example.com/msp"
  export CORE_PEER_TLS_ROOTCERT_FILE="$ORG_BASE/org${N}.example.com/peers/peer0.org${N}.example.com/tls/ca.crt"
}

echo "=== peer channel membership ==="
for N in 1 2 3; do
  setorg "$N"
  chans="$(peer channel list 2>/dev/null | tail -n +2 | paste -sd' ' -)"
  echo "Org${N}: [${chans}]"
done

echo
echo "=== installed chaincode (Org1 peer) ==="
setorg 1
peer lifecycle chaincode queryinstalled 2>&1 | head -10

echo
echo "=== committed chaincode on ${CHANNEL} ==="
setorg 1
peer lifecycle chaincode querycommitted --channelID "$CHANNEL" 2>&1 | head -20
