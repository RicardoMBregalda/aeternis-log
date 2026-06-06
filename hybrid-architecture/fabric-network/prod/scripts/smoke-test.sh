#!/usr/bin/env bash
# Prove the 3-org network anchors with MAJORITY (2/3) endorsement: invoke
# StoreMerkleRoot endorsed by Org1+Org2, then query it back. Runs inside CLI.
set -e
CHANNEL=logchannel
CC=logchaincode
ORDERER=orderer0.example.com:7050
ORDERER_CA=/opt/org/ordererOrganizations/example.com/orderers/orderer0.example.com/tls/ca.crt
ORG_BASE=/opt/org/peerOrganizations
peerca() { echo "$ORG_BASE/org${1}.example.com/peers/peer0.org${1}.example.com/tls/ca.crt"; }

export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID=Org1MSP
export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org1.example.com/users/Admin@org1.example.com/msp"
export CORE_PEER_TLS_ROOTCERT_FILE="$(peerca 1)"

echo "=== invoke StoreMerkleRoot endorsed by Org1 + Org2 (2 of 3) ==="
peer chaincode invoke -o "$ORDERER" --tls --cafile "$ORDERER_CA" -C "$CHANNEL" -n "$CC" \
  --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles "$(peerca 1)" \
  --peerAddresses peer0.org2.example.com:7051 --tlsRootCertFiles "$(peerca 2)" \
  -c '{"function":"StoreMerkleRoot","Args":["batch-smoke-1","deadbeefroot","2026-06-05T00:00:00Z","3","[\"id1\",\"id2\",\"id3\"]"]}' \
  --waitForEvent
sleep 2

echo "=== query the batch back from peer0.org3 (read from a third org) ==="
export CORE_PEER_LOCALMSPID=Org3MSP
export CORE_PEER_ADDRESS=peer0.org3.example.com:7051
export CORE_PEER_MSPCONFIGPATH="$ORG_BASE/org3.example.com/users/Admin@org3.example.com/msp"
export CORE_PEER_TLS_ROOTCERT_FILE="$(peerca 3)"
peer chaincode query -C "$CHANNEL" -n "$CC" -c '{"function":"QueryMerkleBatch","Args":["batch-smoke-1"]}'
