#!/bin/bash

# Set the path to the configtxgen and cryptogen binaries
export PATH=${PWD}/bin:$PATH
export FABRIC_CFG_PATH=${PWD}

# Clean up old artifacts to ensure a fresh generation
rm -rf crypto-config
rm -rf config
mkdir config

# Generate the cryptographic material (certificates and keys)
echo "Generating cryptographic material using cryptogen..."
cryptogen generate --config=./crypto-config.yaml

# cryptogen writes private keys as 0600 owned by the generating user. The API
# container runs as a non-root user and reads its gateway identity key from a
# read-only bind mount, so make the keys group/other-readable. These are local
# dev identities; in production provision the signing key as a secret owned by
# the runtime user instead of loosening permissions.
find crypto-config -name priv_sk -exec chmod 0644 {} +

# Generate the orderer service genesis block
echo "Generating the genesis block..."
configtxgen -profile OneOrgOrdererGenesis -outputBlock ./config/genesis.block -channelID system-channel

# Generate the channel creation transaction
echo "Generating the channel creation transaction..."
configtxgen -profile OneOrgChannel -outputCreateChannelTx ./config/logchannel.tx -channelID logchannel

echo "Artifact generation complete."
