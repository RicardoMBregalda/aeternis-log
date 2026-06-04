#!/bin/bash

# ============================================
# Fabric Network Stop Script
# ============================================
#
# Stops all containers and optionally removes volumes.
#
# Usage: ./stop-network.sh [option]
#
# Options:
#   --clean     Stop and remove all volumes (data will be lost)
#   (none)      Only stop the containers (keeps volumes)
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_success() {
    echo -e "${GREEN}[OK] $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}[WARNING] $1${NC}"
}

print_info() {
    echo -e "${BLUE}[INFO] $1${NC}"
}

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Check the --clean option
if [ "$1" == "--clean" ]; then
    print_warning "The --clean mode will remove ALL volumes."
    print_warning "All blockchain data will be lost."
    echo ""
    read -p "Are you sure you want to continue? (y/N): " -n 1 -r
    echo ""

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_info "Operation cancelled"
        exit 0
    fi

    echo ""
    print_info "Stopping containers and removing volumes..."
    docker-compose down -v
    print_success "Containers stopped and volumes removed"

    # Clean up local artifacts
    echo ""
    print_info "Cleaning up local artifacts..."
    rm -rf crypto-config/ 2>/dev/null || true
    rm -f config/genesis.block config/logchannel.tx 2>/dev/null || true
    rm -f *.block *.tar.gz 2>/dev/null || true
    print_success "Local artifacts removed"

else
    print_info "Stopping containers..."
    docker-compose down
    print_success "Containers stopped (volumes kept)"

    echo ""
    print_info "To also remove volumes, use: ./stop-network.sh --clean"
fi

echo ""
print_success "Fabric network stopped successfully"
