#!/usr/bin/env bash
# Phase G (hardening / DR): back up the AUTHORITATIVE ledger of every orderer and
# peer to timestamped tar.gz archives. The orderer + peer block stores are the
# source of truth; CouchDB state is derived and rebuilt from the ledger on startup,
# so it is not backed up here.
#
# RUNS ON THE HOST. Uses a throwaway alpine container with --volumes-from so it
# works regardless of the compose project's volume-name prefix. Output:
#   prod/backups/<timestamp>/{orderer0,orderer1,orderer2,peer0-org1,...}.tar.gz
#
# NOTE: this is a LIVE (crash-consistent) backup — fine for staging/DR drills.
# For a strictly consistent backup, stop the node first (docker stop <c>), run the
# tar, then start it again; with Raft you can stop one orderer at a time.
#
# Restore (into a fresh, stopped node volume):
#   docker run --rm --volumes-from <container> -v <backupdir>:/backup alpine \
#     sh -c 'rm -rf /var/hyperledger/production/* && tar xzf /backup/<node>.tar.gz -C /var/hyperledger/production'
set -euo pipefail

PROD="/root/tcc-log-management/hybrid-architecture/fabric-network/prod"
TS="$(date +%Y%m%d-%H%M%S)"
BACKUP="$PROD/backups/$TS"
mkdir -p "$BACKUP"

backup_node() { # <container> <ledger_path> <label>
  local container="$1" path="$2" label="$3"
  if ! docker ps --format '{{.Names}}' | grep -qx "$container"; then
    echo "    SKIP $label: container $container not running"
    return
  fi
  echo ">>> $label  ($container:$path)"
  docker run --rm --volumes-from "$container" -v "$BACKUP:/backup" alpine \
    tar czf "/backup/${label}.tar.gz" -C "$path" . 2>/dev/null
}

echo "=== backing up orderer ledgers ==="
for i in 0 1 2; do
  backup_node "orderer${i}.prod" /var/hyperledger/production/orderer "orderer${i}"
done

echo "=== backing up peer ledgers ==="
for n in 1 2 3; do
  backup_node "peer0.org${n}.prod" /var/hyperledger/production "peer0-org${n}"
done

echo
echo "=== backup manifest: $BACKUP ==="
ls -lh "$BACKUP"
echo
echo "Done. CouchDB state is derived from these ledgers and rebuilt on peer startup."
