#!/bin/bash

# Set the path to the configtxgen and cryptogen binaries
export PATH=${PWD}/bin:$PATH
export FABRIC_CFG_PATH=${PWD}

# Clean up old artifacts to ensure a fresh generation
rm -rf crypto-config
rm -rf config
mkdir config

# Generate the cryptographic material (certificates and keys)
echo "####### Generating cryptographic material using cryptogen... #######"
cryptogen generate --config=./crypto-config.yaml

# Generate the orderer service genesis block
echo "####### Generating the Genesis Block... #######"
configtxgen -profile OneOrgOrdererGenesis -outputBlock ./config/genesis.block -channelID system-channel

# Generate the channel creation transaction
echo "####### Generating the channel creation transaction... #######"
configtxgen -profile OneOrgChannel -outputCreateChannelTx ./config/logchannel.tx -channelID logchannel

echo "####### Artifact generation complete! #######"
