#!/bin/bash

# Automatic initialization script for the Fabric network
# This script runs automatically when the CLI container starts

set -e

CHANNEL_NAME="logchannel"
CHAINCODE_NAME="logchaincode"
CHAINCODE_VERSION="1.0"
SEQUENCE=1

# Working directory
cd /opt/gopath/src/github.com/hyperledger/fabric/peer

# Wait for the services to be ready
echo "Waiting for the Fabric network services to become ready..."
sleep 10

# Global configuration
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID='Org1MSP'
export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp

# ============================================
# CHECK IF CHANNEL ALREADY EXISTS
# ============================================
echo "Checking if the channel already exists..."
export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

if peer channel list 2>/dev/null | grep -q "$CHANNEL_NAME"; then
    echo "Channel $CHANNEL_NAME already exists"
else
    echo "Creating channel $CHANNEL_NAME..."

    # Create the artifacts directory if it does not exist
    mkdir -p channel-artifacts

    # Copy the transaction file if it does not exist
    if [ ! -f "channel-artifacts/logchannel.tx" ]; then
        cp config/logchannel.tx channel-artifacts/
    fi

    # Create the channel
    peer channel create \
        -o orderer.example.com:7050 \
        -c $CHANNEL_NAME \
        -f ./channel-artifacts/logchannel.tx \
        --tls \
        --cafile $ORDERER_CA

    echo "Channel created successfully"
    sleep 3
fi

# ============================================
# JOIN PEERS TO THE CHANNEL
# ============================================
echo "Joining peers to the channel..."

# Peer0
echo "  - Joining peer0..."
export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
if ! peer channel list 2>/dev/null | grep -q "$CHANNEL_NAME"; then
    peer channel join -b logchannel.block
    echo "  peer0 joined the channel"
else
    echo "  peer0 is already in the channel"
fi
sleep 2

# Peer1
echo "  - Joining peer1..."
export CORE_PEER_ADDRESS=peer1.org1.example.com:9051
export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer1.org1.example.com/tls/ca.crt
if ! peer channel list 2>/dev/null | grep -q "$CHANNEL_NAME"; then
    peer channel join -b logchannel.block
    echo "  peer1 joined the channel"
else
    echo "  peer1 is already in the channel"
fi
sleep 2

# Peer2
echo "  - Joining peer2..."
export CORE_PEER_ADDRESS=peer2.org1.example.com:11051
export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer2.org1.example.com/tls/ca.crt
if ! peer channel list 2>/dev/null | grep -q "$CHANNEL_NAME"; then
    peer channel join -b logchannel.block
    echo "  peer2 joined the channel"
else
    echo "  peer2 is already in the channel"
fi
sleep 2

# ============================================
# INSTALL CHAINCODE
# ============================================
echo "Checking installed chaincode..."

# Back to peer0
export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

# Check whether the chaincode is already installed
if peer lifecycle chaincode queryinstalled 2>/dev/null | grep -q "logchaincode_1"; then
    echo "Chaincode already installed, retrieving Package ID..."
    peer lifecycle chaincode queryinstalled > /tmp/installed.txt
    export CC_PACKAGE_ID=$(sed -n "/logchaincode_1/{s/^Package ID: //; s/, Label:.*$//; p;}" /tmp/installed.txt)
    echo "  Package ID: $CC_PACKAGE_ID"
else
    echo "Packaging chaincode..."
    peer lifecycle chaincode package logchaincode.tar.gz \
        --path /opt/gopath/src/github.com/chaincode \
        --lang golang \
        --label logchaincode_1

    echo "Installing chaincode on the peers..."

    # Peer0
    echo "  - Installing on peer0..."
    export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    peer lifecycle chaincode install logchaincode.tar.gz

    # Peer1
    echo "  - Installing on peer1..."
    export CORE_PEER_ADDRESS=peer1.org1.example.com:9051
    export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer1.org1.example.com/tls/ca.crt
    peer lifecycle chaincode install logchaincode.tar.gz

    # Peer2
    echo "  - Installing on peer2..."
    export CORE_PEER_ADDRESS=peer2.org1.example.com:11051
    export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer2.org1.example.com/tls/ca.crt
    peer lifecycle chaincode install logchaincode.tar.gz

    echo "Chaincode installed on all peers"

    # Get the Package ID
    export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
    peer lifecycle chaincode queryinstalled > /tmp/installed.txt
    export CC_PACKAGE_ID=$(sed -n "/logchaincode_1/{s/^Package ID: //; s/, Label:.*$//; p;}" /tmp/installed.txt)
    echo "  Package ID: $CC_PACKAGE_ID"
fi

# ============================================
# APPROVE CHAINCODE
# ============================================
echo "Checking chaincode approval..."

if peer lifecycle chaincode checkcommitreadiness \
    -C $CHANNEL_NAME \
    -n $CHAINCODE_NAME \
    -v $CHAINCODE_VERSION \
    --sequence $SEQUENCE \
    --tls \
    --cafile $ORDERER_CA \
    --output json 2>/dev/null | grep -q '"Org1MSP": true'; then
    echo "Chaincode already approved by Org1MSP"
else
    echo "Approving chaincode for Org1MSP..."
    peer lifecycle chaincode approveformyorg \
        -o orderer.example.com:7050 \
        --tls \
        --cafile $ORDERER_CA \
        -C $CHANNEL_NAME \
        -n $CHAINCODE_NAME \
        -v $CHAINCODE_VERSION \
        --package-id $CC_PACKAGE_ID \
        --sequence $SEQUENCE

    echo "Chaincode approved"
    sleep 3
fi

# ============================================
# COMMIT CHAINCODE
# ============================================
echo "Checking chaincode commit..."

if peer lifecycle chaincode querycommitted -C $CHANNEL_NAME 2>/dev/null | grep -q "$CHAINCODE_NAME"; then
    echo "Chaincode is already committed on the channel"
else
    echo "Committing chaincode..."
    peer lifecycle chaincode commit \
        -o orderer.example.com:7050 \
        --tls \
        --cafile $ORDERER_CA \
        -C $CHANNEL_NAME \
        -n $CHAINCODE_NAME \
        -v $CHAINCODE_VERSION \
        --sequence $SEQUENCE \
        --peerAddresses peer0.org1.example.com:7051 \
        --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

    echo "Chaincode committed successfully"
    sleep 3
fi

# ============================================
# FINAL CHECK
# ============================================
echo ""
echo "=========================================="
echo "Fabric network configured successfully"
echo "=========================================="
echo ""
echo "Network status:"
echo "  - Channel: $CHANNEL_NAME"
echo "  - Chaincode: $CHAINCODE_NAME v$CHAINCODE_VERSION"
echo "  - Peers: peer0, peer1, peer2"
echo ""
echo "To test, run:"
echo "  docker exec cli peer chaincode query -C $CHANNEL_NAME -n $CHAINCODE_NAME -c '{\"Args\":[\"queryAllLogs\"]}'"
echo ""
echo "=========================================="

# Keep the container running
echo "CLI container ready for use. Press Ctrl+C to exit."
tail -f /dev/null
