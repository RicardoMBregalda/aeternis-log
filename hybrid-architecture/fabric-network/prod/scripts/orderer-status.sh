#!/usr/bin/env bash
# Read-only: list channel membership on each of the 3 orderers via the channel
# participation (osnadmin) admin endpoint. Runs inside the CLI container, using the
# orderer org TLS material (mounted at /opt/org/ordererOrganizations).
set +e

OORG=/opt/org/ordererOrganizations/example.com
# orderer0's TLS leaf works as an admin mutual-TLS client cert against all 3
# (all signed by the same orderer-org CA).
CLI_CERT="$OORG/orderers/orderer0.example.com/tls/server.crt"
CLI_KEY="$OORG/orderers/orderer0.example.com/tls/server.key"
CA="$OORG/orderers/orderer0.example.com/tls/ca.crt"

for i in 0 1 2; do
  echo "=== orderer${i}.example.com:7053 ==="
  osnadmin channel list -o "orderer${i}.example.com:7053" \
    --ca-file "$CA" --client-cert "$CLI_CERT" --client-key "$CLI_KEY" 2>&1 | head -20
  echo
done
