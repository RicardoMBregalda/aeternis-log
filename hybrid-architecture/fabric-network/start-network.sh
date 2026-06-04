#!/bin/bash

# ============================================
# Master Script - Fabric Network Startup
# ============================================
#
# This script orchestrates the entire Hyperledger Fabric network startup process.
# It automatically runs all the required steps in the correct order.
#
# Usage: ./start-network.sh [option]
#
# Options:
#   --clean     Wipe everything and start from scratch
#   --restart   Restart the network without recreating artifacts
#   (none)      Normal startup (default)
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Helper to print in color
print_header() {
    echo -e "\n${BLUE}=========================================="
    echo -e "$1"
    echo -e "==========================================${NC}\n"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

# Parse arguments
CLEAN_MODE=false
RESTART_MODE=false

if [ "$1" == "--clean" ]; then
    CLEAN_MODE=true
    print_warning "CLEAN mode: removing everything and recreating from scratch"
elif [ "$1" == "--restart" ]; then
    RESTART_MODE=true
    print_info "RESTART mode: restarting without recreating artifacts"
fi

# ============================================
# STEP 1: CLEAN (if --clean)
# ============================================
if [ "$CLEAN_MODE" = true ]; then
    print_header "STEP 1: FULL CLEANUP"

    echo "🗑️  Stopping and removing containers..."
    docker-compose down -v 2>/dev/null || true

    echo "🗑️  Removing old artifacts..."
    rm -rf crypto-config/ 2>/dev/null || true
    rm -f config/genesis.block config/logchannel.tx 2>/dev/null || true
    rm -f *.block *.tar.gz 2>/dev/null || true

    print_success "Full cleanup complete"
    sleep 2
fi

# ============================================
# STEP 2: CHECK/GENERATE ARTIFACTS
# ============================================
if [ "$RESTART_MODE" = false ]; then
    print_header "STEP 2: CRYPTOGRAPHIC ARTIFACTS"

    if [ -f "config/genesis.block" ] && [ -f "config/logchannel.tx" ] && [ -d "crypto-config" ]; then
        print_info "Artifacts already exist, skipping generation..."
    else
        print_info "Generating network certificates and artifacts..."
        chmod +x scripts/1-generate-artifacts.sh
        ./scripts/1-generate-artifacts.sh
        print_success "Artifacts generated successfully"
    fi

    # Check that the artifacts were created
    if [ ! -f "config/genesis.block" ] || [ ! -f "config/logchannel.tx" ]; then
        print_error "Failed to generate artifacts!"
        exit 1
    fi

    sleep 2
fi

# ============================================
# STEP 3: START CONTAINERS
# ============================================
print_header "STEP 3: START DOCKER CONTAINERS"

if [ "$RESTART_MODE" = true ]; then
    echo "🔄 Restarting containers..."
    docker-compose restart
else
    echo "🚀 Starting Docker Compose containers..."
    docker-compose up -d
fi

# Wait for the containers to become healthy
echo "⏳ Waiting for containers to start..."
sleep 15

# Check status
RUNNING=$(docker-compose ps --services --filter "status=running" | wc -l)
TOTAL=$(docker-compose ps --services | wc -l)

if [ "$RUNNING" -lt "$((TOTAL - 2))" ]; then
    print_warning "Some containers may not be running correctly"
    docker-compose ps
else
    print_success "All containers started ($RUNNING/$TOTAL running)"
fi

sleep 2

# ============================================
# STEP 4: WAIT FOR AUTOMATIC INITIALIZATION
# ============================================
print_header "STEP 4: AUTOMATIC NETWORK CONFIGURATION"

print_info "The CLI container is running the automatic initialization..."
print_info "This includes:"
echo "  • Creating the 'logchannel' channel"
echo "  • Joining the peers to the channel"
echo "  • Installing the chaincode"
echo "  • Approving and committing the chaincode"
echo ""

# Wait for automatic initialization (by checking the logs)
echo "⏳ Waiting for automatic configuration (30-60s)..."

for i in {1..60}; do
    if docker logs cli 2>&1 | grep -q "FABRIC NETWORK CONFIGURED SUCCESSFULLY"; then
        print_success "Automatic initialization complete!"
        break
    fi

    if [ $i -eq 60 ]; then
        print_warning "Timeout reached, checking status manually..."
    fi

    sleep 1
done

sleep 2

# ============================================
# STEP 5: VERIFICATION
# ============================================
print_header "STEP 5: STATUS VERIFICATION"

echo "📊 Checking the channel..."
if docker exec cli peer channel list 2>/dev/null | grep -q "logchannel"; then
    print_success "Channel 'logchannel' created and peers connected"
else
    print_error "Channel was not created correctly"
fi

echo ""
echo "📦 Checking the chaincode..."
if docker exec cli peer lifecycle chaincode querycommitted -C logchannel 2>/dev/null | grep -q "logchaincode"; then
    print_success "Chaincode 'logchaincode' installed and committed"
else
    print_warning "Chaincode may not be fully configured"
fi

sleep 2

# ============================================
# FINAL SUMMARY
# ============================================
print_header "✅ FABRIC NETWORK READY!"

echo "📊 Component Status:"
echo ""
docker-compose ps --format "table {{.Name}}\t{{.Status}}" | grep -E "(NAME|Up)" || docker-compose ps
echo ""

print_info "Access URLs:"
echo "  • CouchDB Peer0: http://localhost:5984/_utils (admin/password)"
echo "  • CouchDB Peer1: http://localhost:6984/_utils (admin/password)"
echo "  • CouchDB Peer2: http://localhost:7984/_utils (admin/password)"
echo ""

print_info "Useful Commands:"
echo "  • View CLI logs:       docker logs cli -f"
echo "  • Test chaincode:      ./test-network.sh"
echo "  • Stop network:        docker-compose down"
echo "  • Restart:             ./start-network.sh --restart"
echo "  • Full reset:          ./start-network.sh --clean"
echo ""

print_header "🎉 INITIALIZATION COMPLETED SUCCESSFULLY!"

# Ask whether to run tests
echo ""
read -p "Do you want to run the chaincode tests now? (y/N): " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    print_info "Running tests..."
    ./test-network.sh
fi
