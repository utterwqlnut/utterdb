#!/usr/bin/env bash
# Run on the benchmark instance (same subnet as the cluster).
# Usage: start_benchmark.sh <nlb_private_ip> <nlb_port> [users] [spawn_rate] [duration]
set -euo pipefail

NLB_IP=${1:?nlb_private_ip required}
NLB_PORT=${2:-9000}
USERS=${3:-50}
SPAWN_RATE=${4:-10}
DURATION=${5:-60s}
APP_DIR=/opt/utterdb

# ── Install Locust if needed ──────────────────────────────────────────────────
if ! command -v locust &>/dev/null; then
  echo "Installing Locust..."
  dnf install -y -q python3 python3-pip
  python3 -m pip install -q --ignore-installed locust
fi

echo "========================================="
echo " utterdb benchmark (Locust)"
echo " target   : ${NLB_IP}:${NLB_PORT}"
echo " users    : ${USERS}  spawn_rate: ${SPAWN_RATE}/s  duration: ${DURATION}"
echo "========================================="

cd "$APP_DIR"

locust -f src/test/benchmark.py \
  --headless \
  --host "tcp://${NLB_IP}:${NLB_PORT}" \
  -u "$USERS" \
  -r "$SPAWN_RATE" \
  --run-time "$DURATION" \
  --processes -1
