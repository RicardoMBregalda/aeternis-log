#!/usr/bin/env bash
# Provision identities for the production-staging network using Fabric CA
# (register/enroll) — no cryptogen. One CA per org; this builds the MSP and TLS
# material under organizations/{peer,orderer}Organizations.
#
# Run AFTER the CAs are up (docker-compose-ca.yml). Idempotent enough to re-run
# after a clean (rm -rf organizations/peerOrganizations organizations/ordererOrganizations).
set -euo pipefail

PROD="/root/aeternis-log/hybrid-architecture/fabric-network/prod"
BIN="/root/aeternis-log/hybrid-architecture/fabric-network/bin"
FCA="$BIN/fabric-ca-client"
ORGS="$PROD/organizations"
PEERORGS="$ORGS/peerOrganizations"
ORDORGS="$ORGS/ordererOrganizations"

# ---- helpers --------------------------------------------------------------

# write_nodeou_config <msp_dir> <cacert_filename>
write_nodeou_config() {
  local mspdir="$1" cacert="$2"
  cat > "$mspdir/config.yaml" <<EOF
NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/$cacert
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/$cacert
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/$cacert
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/$cacert
    OrganizationalUnitIdentifier: orderer
EOF
}

# normalize_tls <node_tls_dir> : rename CA-issued TLS files to server.crt/key/ca.crt
normalize_tls() {
  local tlsdir="$1"
  cp "$tlsdir"/tlscacerts/* "$tlsdir/ca.crt"
  cp "$tlsdir"/signcerts/*  "$tlsdir/server.crt"
  cp "$tlsdir"/keystore/*   "$tlsdir/server.key"
}

# ---- peer org -------------------------------------------------------------
# createPeerOrg <orgnum> <ca_host_port> <caname>
createPeerOrg() {
  local N="$1" PORT="$2" CANAME="$3"
  local DOMAIN="org${N}.example.com"
  local ORGDIR="$PEERORGS/$DOMAIN"
  local CATLS="$ORGS/fabric-ca/org${N}/tls-cert.pem"
  echo ">>> Peer org ${N} (${DOMAIN}) via ${CANAME} on :${PORT}"

  mkdir -p "$ORGDIR"
  export FABRIC_CA_CLIENT_HOME="$ORGDIR"

  # CA admin enroll -> establishes org msp/cacerts
  "$FCA" enroll -u "https://admin:adminpw@localhost:${PORT}" --caname "$CANAME" \
    --tls.certfiles "$CATLS" >/dev/null
  local CACERT
  CACERT="$(basename "$(ls "$ORGDIR"/msp/cacerts/*.pem | head -1)")"
  write_nodeou_config "$ORGDIR/msp" "$CACERT"
  # org-level tlscacerts (for configtx): the TLS CA root that signs node TLS certs.
  # In this single-CA fabric-ca setup it equals the identity root ca-cert.pem
  # (CA:TRUE); the server's tls-cert.pem is a leaf (CA:FALSE) and must NOT be used.
  mkdir -p "$ORGDIR/msp/tlscacerts"
  cp "$ORGS/fabric-ca/org${N}/ca-cert.pem" "$ORGDIR/msp/tlscacerts/ca.crt"

  # register node + identities
  "$FCA" register --caname "$CANAME" --id.name "peer0"    --id.secret "peer0pw"  --id.type peer   --tls.certfiles "$CATLS" >/dev/null
  "$FCA" register --caname "$CANAME" --id.name "user1"    --id.secret "user1pw"  --id.type client --tls.certfiles "$CATLS" >/dev/null
  "$FCA" register --caname "$CANAME" --id.name "orgadmin" --id.secret "adminpw"  --id.type admin  --tls.certfiles "$CATLS" >/dev/null

  # peer0 MSP
  local PEER="$ORGDIR/peers/peer0.$DOMAIN"
  "$FCA" enroll -u "https://peer0:peer0pw@localhost:${PORT}" --caname "$CANAME" \
    -M "$PEER/msp" --csr.hosts "peer0.$DOMAIN" --tls.certfiles "$CATLS" >/dev/null
  write_nodeou_config "$PEER/msp" "$CACERT"

  # peer0 TLS
  "$FCA" enroll -u "https://peer0:peer0pw@localhost:${PORT}" --caname "$CANAME" \
    -M "$PEER/tls" --enrollment.profile tls \
    --csr.hosts "peer0.$DOMAIN" --csr.hosts localhost --tls.certfiles "$CATLS" >/dev/null
  normalize_tls "$PEER/tls"

  # user1 MSP
  local USER="$ORGDIR/users/User1@$DOMAIN"
  "$FCA" enroll -u "https://user1:user1pw@localhost:${PORT}" --caname "$CANAME" \
    -M "$USER/msp" --tls.certfiles "$CATLS" >/dev/null
  write_nodeou_config "$USER/msp" "$CACERT"

  # admin MSP
  local ADMIN="$ORGDIR/users/Admin@$DOMAIN"
  "$FCA" enroll -u "https://orgadmin:adminpw@localhost:${PORT}" --caname "$CANAME" \
    -M "$ADMIN/msp" --tls.certfiles "$CATLS" >/dev/null
  write_nodeou_config "$ADMIN/msp" "$CACERT"

  echo "    done: $DOMAIN"
}

# ---- orderer org ----------------------------------------------------------
createOrdererOrg() {
  local PORT="7454" CANAME="ca-orderer"
  local DOMAIN="example.com"
  local ORGDIR="$ORDORGS/$DOMAIN"
  local CATLS="$ORGS/fabric-ca/ordererOrg/tls-cert.pem"
  echo ">>> Orderer org (${DOMAIN}) via ${CANAME} on :${PORT}"

  mkdir -p "$ORGDIR"
  export FABRIC_CA_CLIENT_HOME="$ORGDIR"

  "$FCA" enroll -u "https://admin:adminpw@localhost:${PORT}" --caname "$CANAME" \
    --tls.certfiles "$CATLS" >/dev/null
  local CACERT
  CACERT="$(basename "$(ls "$ORGDIR"/msp/cacerts/*.pem | head -1)")"
  write_nodeou_config "$ORGDIR/msp" "$CACERT"
  # TLS CA root (CA:TRUE) — same as identity root in this single-CA setup.
  mkdir -p "$ORGDIR/msp/tlscacerts"
  cp "$ORGS/fabric-ca/ordererOrg/ca-cert.pem" "$ORGDIR/msp/tlscacerts/tlsca.$DOMAIN-cert.pem"

  # register 3 orderers + admin
  "$FCA" register --caname "$CANAME" --id.name "orderer0"     --id.secret "ordererpw" --id.type orderer --tls.certfiles "$CATLS" >/dev/null
  "$FCA" register --caname "$CANAME" --id.name "orderer1"     --id.secret "ordererpw" --id.type orderer --tls.certfiles "$CATLS" >/dev/null
  "$FCA" register --caname "$CANAME" --id.name "orderer2"     --id.secret "ordererpw" --id.type orderer --tls.certfiles "$CATLS" >/dev/null
  "$FCA" register --caname "$CANAME" --id.name "ordereradmin" --id.secret "adminpw"   --id.type admin   --tls.certfiles "$CATLS" >/dev/null

  for i in 0 1 2; do
    local OHOST="orderer${i}.$DOMAIN"
    local ODIR="$ORGDIR/orderers/$OHOST"
    "$FCA" enroll -u "https://orderer${i}:ordererpw@localhost:${PORT}" --caname "$CANAME" \
      -M "$ODIR/msp" --csr.hosts "$OHOST" --tls.certfiles "$CATLS" >/dev/null
    write_nodeou_config "$ODIR/msp" "$CACERT"
    "$FCA" enroll -u "https://orderer${i}:ordererpw@localhost:${PORT}" --caname "$CANAME" \
      -M "$ODIR/tls" --enrollment.profile tls \
      --csr.hosts "$OHOST" --csr.hosts localhost --tls.certfiles "$CATLS" >/dev/null
    normalize_tls "$ODIR/tls"
    echo "    done: $OHOST"
  done

  # orderer org admin
  local ADMIN="$ORGDIR/users/Admin@$DOMAIN"
  "$FCA" enroll -u "https://ordereradmin:adminpw@localhost:${PORT}" --caname "$CANAME" \
    -M "$ADMIN/msp" --tls.certfiles "$CATLS" >/dev/null
  write_nodeou_config "$ADMIN/msp" "$CACERT"

  echo "    done: orderer org"
}

# ---- run ------------------------------------------------------------------
createPeerOrg 1 7154 ca-org1
createPeerOrg 2 7254 ca-org2
createPeerOrg 3 7354 ca-org3
createOrdererOrg

echo
echo "Identity provisioning complete. Material under: $ORGS"
