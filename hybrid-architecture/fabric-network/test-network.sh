#!/bin/bash

# ============================================
# Fabric Network Test Script
# ============================================
#
# Runs quick tests to validate that the network is working correctly.
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_header() {
    echo -e "\n${BLUE}=========================================="
    echo -e "$1"
    echo -e "==========================================${NC}\n"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

# ============================================
# TESTS
# ============================================
print_header "🧪 FABRIC NETWORK TESTS"

# Test 1: CLI container responding
print_info "Test 1: Checking the CLI container..."
if docker exec cli echo "OK" &>/dev/null; then
    print_success "CLI container is responding"
else
    print_error "CLI container is not reachable"
    exit 1
fi

# Test 2: Channel created
print_info "Test 2: Checking the logchannel channel..."
if docker exec cli peer channel list 2>/dev/null | grep -q "logchannel"; then
    print_success "Channel 'logchannel' exists"
else
    print_error "Channel 'logchannel' not found"
    exit 1
fi

# Test 3: Chaincode installed
print_info "Test 3: Checking installed chaincode..."
if docker exec cli peer lifecycle chaincode queryinstalled 2>/dev/null | grep -q "logchaincode"; then
    print_success "Chaincode 'logchaincode' is installed"
else
    print_error "Chaincode is not installed"
    exit 1
fi

# Test 4: Chaincode committed
print_info "Test 4: Checking committed chaincode..."
if docker exec cli peer lifecycle chaincode querycommitted -C logchannel 2>/dev/null | grep -q "logchaincode"; then
    print_success "Chaincode is committed on the channel"
else
    print_error "Chaincode is not committed"
    exit 1
fi

# Test 5: Write transaction (invoke)
print_info "Test 5: Testing a write transaction..."
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LOG_ID="TEST_$(date +%s)"

docker exec cli bash -c "
    export CORE_PEER_TLS_ENABLED=true
    export CORE_PEER_LOCALMSPID='Org1MSP'
    export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
    export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

    peer chaincode invoke \
        -C logchannel \
        -n logchaincode \
        -c '{\"Args\":[\"CreateLog\",\"$LOG_ID\",\"hash123\",\"$TIMESTAMP\",\"test-script\",\"INFO\",\"Automated network test\",\"{}\",\"\"]}' \
        --tls \
        --cafile \$ORDERER_CA
" &>/dev/null

if [ $? -eq 0 ]; then
    print_success "Write transaction executed successfully"
else
    print_error "Write transaction failed"
    exit 1
fi

# Wait for the transaction to be processed
sleep 3

# Test 6: Read transaction (query)
print_info "Test 6: Testing a read transaction..."
QUERY_RESULT=$(docker exec cli bash -c "
    export CORE_PEER_TLS_ENABLED=true
    export CORE_PEER_LOCALMSPID='Org1MSP'
    export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
    export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
    export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt

    peer chaincode query \
        -C logchannel \
        -n logchaincode \
        -c '{\"Args\":[\"QueryLog\",\"$LOG_ID\"]}'
" 2>&1)

if echo "$QUERY_RESULT" | grep -q "$LOG_ID"; then
    print_success "Read transaction executed successfully"
    echo ""
    echo "📄 Query result:"
    echo "$QUERY_RESULT" | jq '.' 2>/dev/null || echo "$QUERY_RESULT"
else
    print_error "Read transaction failed"
    echo "$QUERY_RESULT"
    exit 1
fi

# ============================================
# SUMMARY
# ============================================
print_header "✅ ALL TESTS PASSED!"

echo "Network Statistics:"
echo ""
echo "📊 Containers:"
docker-compose ps --format "table {{.Name}}\t{{.Status}}" | grep "Up" | wc -l | xargs echo "  • Running:"
echo ""
echo "🔗 Channel:"
echo "  • Name: logchannel"
docker exec cli peer channel getinfo -c logchannel 2>/dev/null | grep "Blockchain info:" | sed 's/^/  • /'
echo ""
echo "📦 Chaincode:"
docker exec cli peer lifecycle chaincode querycommitted -C logchannel 2>/dev/null | grep "Name:" | sed 's/^/  • /'
echo ""

print_info "The Hyperledger Fabric network is fully operational! 🚀"
