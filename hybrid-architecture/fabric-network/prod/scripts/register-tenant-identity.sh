#!/usr/bin/env bash
# Provision a per-tenant Fabric identity for the API (F14). The identity carries
# a CA-signed `tenant` attribute in its enrollment certificate; the chaincode
# derives the tenant from that attribute (never from a call argument) and
# partitions Merkle-batch state by it. Anchoring a tenant's batches with this
# identity is what makes the ledger-level isolation real end to end.
#
# The output is a least-privilege MSP bundle (signcert + key 0400 + peer TLS CA)
# the API mounts as that tenant's gateway identity — mirrors build-api-identity.sh.
#
# RUNS ON THE HOST. Usage: register-tenant-identity.sh <tenant> [org_num]
#   <tenant>   logical tenant id, e.g. acme    (becomes the `tenant` ecert attr)
#   [org_num]  peer org whose CA enrolls it    (default 1 — the API's org)
#
# Env (defaults target org1's CA from this repo's prod network):
#   CA_URL        default https://localhost:7054
#   CA_NAME       default ca-org1
#   CA_ADMIN      default admin
#   CA_ADMIN_PW   default adminpw
set -euo pipefail

TENANT="${1:-}"
N="${2:-1}"
[ -n "$TENANT" ] || { echo "usage: register-tenant-identity.sh <tenant> [org_num]" >&2; exit 2; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROD="$(dirname "$SCRIPT_DIR")"                # .../fabric-network/prod
BIN="$(dirname "$PROD")/bin"
FCA="$BIN/fabric-ca-client"
DOMAIN="org${N}.example.com"
ORG_CA_TLS="$PROD/organizations/fabric-ca/org${N}/ca-cert.pem"

CA_URL="${CA_URL:-https://localhost:7054}"
CA_NAME="${CA_NAME:-ca-org${N}}"
CA_ADMIN="${CA_ADMIN:-admin}"
CA_ADMIN_PW="${CA_ADMIN_PW:-adminpw}"

USERNAME="tenant-${TENANT}"
USERPW="${USERNAME}-pw"
DST="$PROD/api-identity/tenants/${TENANT}"
RUNTIME_UID=1000
RUNTIME_GID=1000

echo "=== registering per-tenant identity '${USERNAME}' (tenant=${TENANT}) at ${CA_NAME} ==="

# 1) Enroll the CA admin so we have registrar rights.
export FABRIC_CA_CLIENT_HOME="$(mktemp -d)"
"$FCA" enroll -u "https://${CA_ADMIN}:${CA_ADMIN_PW}@${CA_URL#https://}" \
  --caname "$CA_NAME" --tls.certfiles "$ORG_CA_TLS"

# 2) Register the tenant identity with a `tenant` attribute, defaulted into the
#    ecert (':ecert'), so it is present without an explicit request at use time.
"$FCA" register --caname "$CA_NAME" --tls.certfiles "$ORG_CA_TLS" \
  --id.name "$USERNAME" --id.secret "$USERPW" --id.type client \
  --id.attrs "tenant=${TENANT}:ecert" || echo "(already registered — continuing)"

# 3) Enroll it, explicitly requesting the `tenant` attribute into the cert.
ENROLL_MSP="$(mktemp -d)/msp"
"$FCA" enroll -u "https://${USERNAME}:${USERPW}@${CA_URL#https://}" \
  --caname "$CA_NAME" --tls.certfiles "$ORG_CA_TLS" \
  --enrollment.attrs "tenant" -M "$ENROLL_MSP"

# 4) Build the least-privilege bundle the API mounts for this tenant.
rm -rf "$DST"
mkdir -p "$DST/signcerts" "$DST/keystore" "$DST/tls"
cp "$ENROLL_MSP"/signcerts/*                                   "$DST/signcerts/cert.pem"
cp "$(ls "$ENROLL_MSP"/keystore/* | head -1)"                  "$DST/keystore/key.pem"
cp "$PROD/organizations/peerOrganizations/$DOMAIN/peers/peer0.$DOMAIN/tls/ca.crt" "$DST/tls/ca.crt"

chown -R "$RUNTIME_UID:$RUNTIME_GID" "$DST"
chmod 0500 "$DST" "$DST/signcerts" "$DST/keystore" "$DST/tls"
chmod 0444 "$DST/signcerts/cert.pem" "$DST/tls/ca.crt"
chmod 0400 "$DST/keystore/key.pem"

echo "    wrote $DST (owner ${RUNTIME_UID}:${RUNTIME_GID}, key 0400)"
echo "    point the API at this tenant's identity (fabric.tenant_identities['${TENANT}'])"
echo "    and verify: the ecert carries 'tenant=${TENANT}'."
