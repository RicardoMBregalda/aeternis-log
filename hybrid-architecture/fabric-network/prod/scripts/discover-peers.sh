#!/usr/bin/env bash
# Read-only: use service discovery from Org1's vantage point to list the peers and
# the endorsement plan for logchaincode on logchannel. If discovery returns Org2/Org3
# peers, the Fabric Gateway can auto-collect the MAJORITY (2/3) endorsement without
# the client naming peers. Runs inside the CLI container.
set +e

ORG_BASE=/opt/org/peerOrganizations
O1="$ORG_BASE/org1.example.com"
KEY="$(ls "$O1"/users/Admin@org1.example.com/msp/keystore/* | head -1)"
CERT="$O1/users/Admin@org1.example.com/msp/signcerts/cert.pem"
TLSCA="$O1/peers/peer0.org1.example.com/tls/ca.crt"
SERVER=peer0.org1.example.com:7051

echo "=== discovered peers on logchannel ==="
discover --peerTLSCA "$TLSCA" --userKey "$KEY" --userCert "$CERT" --MSP Org1MSP \
  peers --channel logchannel --server "$SERVER" 2>&1 \
  | grep -E '"MSPID"|"Endpoint"' | sed 's/^[[:space:]]*//'

echo
echo "=== endorsement plan for logchaincode ==="
discover --peerTLSCA "$TLSCA" --userKey "$KEY" --userCert "$CERT" --MSP Org1MSP \
  endorsers --channel logchannel --server "$SERVER" --chaincode logchaincode 2>&1 \
  | grep -E '"MSPID"|"Endpoint"|layout|Quantity' | sed 's/^[[:space:]]*//' | head -40
