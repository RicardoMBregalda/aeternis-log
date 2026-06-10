#!/usr/bin/env bash
# Fault-tolerance acceptance tests for the 3-org / Raft-3 network.
# RUNS ON THE HOST (uses `docker stop/start`); drives anchoring via anchor.sh
# inside cli.prod. A trap restarts every container it stops, so a failed test
# never leaves the network degraded.
#
#   1. Raft tolerates 1 orderer down  -> anchor still succeeds.
#   2. Drop 1 peer org (2/3 up)       -> anchor still succeeds (MAJORITY met).
#   3. Drop 2 peer orgs (1/3 up)      -> anchor FAILS (endorsement is real).
set -u

STOPPED=()
restore() {
  echo
  echo "=== restoring stopped containers ==="
  for c in "${STOPPED[@]}"; do docker start "$c" >/dev/null && echo "started $c"; done
}
trap restore EXIT

stop() { docker stop "$1" >/dev/null && STOPPED+=("$1") && echo "stopped $1"; }

anchor() { # batch_id "endorsers" [orderer]
  docker exec cli.prod bash /opt/scripts/anchor.sh "$1" "$2" "${3:-orderer0.example.com:7050}" 2>&1 | tail -3
}

pass() { echo "RESULT: PASS - $1"; }
fail() { echo "RESULT: FAIL - $1"; }

echo "############ TEST 1: Raft tolerates 1 orderer down ############"
stop orderer2.prod
sleep 3
if anchor batch-ft-orderer "1 2" orderer0.example.com:7050 | grep -q "status (VALID)"; then
  pass "anchored with orderer2 down (Raft quorum 2/3)"
else
  fail "could not anchor with 1 orderer down"
fi
docker start orderer2.prod >/dev/null; echo "started orderer2.prod"
# pop orderer2 from STOPPED so the trap doesn't double-start
STOPPED=("${STOPPED[@]/orderer2.prod}")
sleep 3

echo
echo "############ TEST 2: drop 1 peer org -> anchor still succeeds (2/3) ############"
stop peer0.org3.prod
sleep 2
if anchor batch-ft-1org "1 2" | grep -q "status (VALID)"; then
  pass "anchored with org3 down (Org1+Org2 = MAJORITY)"
else
  fail "could not anchor with 1 org down"
fi

echo
echo "############ TEST 3: drop 2 peer orgs -> anchor must FAIL ############"
stop peer0.org2.prod
sleep 2
# Only Org1 is up now; collecting a 2nd endorsement is impossible.
if anchor batch-ft-2org "1 2" | grep -qiE "status \(VALID\)"; then
  fail "anchor SUCCEEDED with 2 orgs down (endorsement not enforced!)"
else
  pass "anchor correctly REJECTED with 2 orgs down (MAJORITY not met)"
fi

echo
echo "All fault-tolerance tests done."
