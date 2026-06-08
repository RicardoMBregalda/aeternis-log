#!/usr/bin/env bash
# Phase G (hardening): build a LEAST-PRIVILEGE identity bundle for the API so it
# can run as a non-root container and mount only its own credentials — not the
# whole organizations/ tree (which holds every org's private keys, including the
# orderers').
#
# Extracts, for one peer org, just: the admin signcert, the admin signing key, and
# the peer TLS CA. Ownership is set to the API container's runtime uid (1000,
# "appuser" in the image) with the private key 0400, so the non-root process can
# read it without a world-readable key on disk.
#
# RUNS ON THE HOST (needs root to chown). Usage: build-api-identity.sh [org_num]
set -euo pipefail

N="${1:-1}"
PROD="/root/aeternis-log/hybrid-architecture/fabric-network/prod"
DOMAIN="org${N}.example.com"
SRC="$PROD/organizations/peerOrganizations/$DOMAIN"
DST="$PROD/api-identity/org${N}"
RUNTIME_UID=1000   # "appuser" in api/Dockerfile
RUNTIME_GID=1000

echo "=== building least-privilege API identity bundle for $DOMAIN ==="
rm -rf "$DST"
mkdir -p "$DST/signcerts" "$DST/keystore" "$DST/tls"

cp "$SRC/users/Admin@$DOMAIN/msp/signcerts/cert.pem"            "$DST/signcerts/cert.pem"
cp "$(ls "$SRC/users/Admin@$DOMAIN/msp/keystore/"* | head -1)"  "$DST/keystore/key.pem"
cp "$SRC/peers/peer0.$DOMAIN/tls/ca.crt"                        "$DST/tls/ca.crt"

# Least-privilege ownership/permissions: runtime uid owns it; key is 0400.
chown -R "$RUNTIME_UID:$RUNTIME_GID" "$DST"
chmod 0500 "$DST" "$DST/signcerts" "$DST/keystore" "$DST/tls"
chmod 0444 "$DST/signcerts/cert.pem" "$DST/tls/ca.crt"
chmod 0400 "$DST/keystore/key.pem"

echo "    wrote $DST (owner ${RUNTIME_UID}:${RUNTIME_GID}, key 0400)"
ls -la "$DST/keystore/"
echo
echo "Mount this bundle (read-only) at /fabric-identity and run the API as uid ${RUNTIME_UID}."
