#!/usr/bin/env bash
# Warm the chaincode container on every Org1 peer (peer0/1/2). Chaincode containers
# start lazily on the first endorsement a peer serves; after a network restart only
# the peer that served traffic has its container up. The Fabric Gateway picks
# endorsers via service discovery, so a cold peer makes endorsement time out. Running
# one query per peer forces each container to start. Runs inside the dev `cli`.
set +e

CRYPTO=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com
CH=logchannel
CC=logchaincode

setpeer() { # <n> <port>
  export CORE_PEER_TLS_ENABLED=true
  export CORE_PEER_LOCALMSPID=Org1MSP
  export CORE_PEER_MSPCONFIGPATH="$CRYPTO/users/Admin@org1.example.com/msp"
  export CORE_PEER_ADDRESS="peer$1.org1.example.com:$2"
  export CORE_PEER_TLS_ROOTCERT_FILE="$CRYPTO/peers/peer$1.org1.example.com/tls/ca.crt"
}

for spec in "0 7051" "1 9051" "2 11051"; do
  set -- $spec
  setpeer "$1" "$2"
  echo "=== peer$1  ($CORE_PEER_ADDRESS) ==="
  echo -n "  installed logchaincode_1: "
  peer lifecycle chaincode queryinstalled 2>/dev/null | grep -c "logchaincode_1"
  echo -n "  warm query: "
  if peer chaincode query -C "$CH" -n "$CC" -c '{"function":"GetAllMerkleBatches","Args":[]}' >/tmp/warm.out 2>/tmp/warm.err; then
    echo "OK ($(wc -c </tmp/warm.out) bytes)"
  else
    echo "FAILED"
    tail -2 /tmp/warm.err
  fi
done
echo "Done."
