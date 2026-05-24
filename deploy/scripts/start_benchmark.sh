#!/usr/bin/env bash
# Run on the benchmark instance (same subnet as the cluster).
# Usage: start_benchmark.sh <nlb_private_ip> <nlb_port> [workers] [duration] [seed]
set -euo pipefail

NLB_IP=${1:?nlb_private_ip required}
NLB_PORT=${2:-9000}
WORKERS=${3:-50}
DURATION=${4:-60s}
SEED=${5:-500}
APP_DIR=/opt/utterdb

export PATH=$PATH:/usr/local/go/bin:/root/go/bin

cd "$APP_DIR"

echo "========================================="
echo " utterdb benchmark"
echo " target : ${NLB_IP}:${NLB_PORT}"
echo " workers: ${WORKERS}  duration: ${DURATION}  seed: ${SEED}"
echo "========================================="

go run src/test/benchmark.go \
  --host    "$NLB_IP" \
  --port    "$NLB_PORT" \
  --workers "$WORKERS" \
  --duration "$DURATION" \
  --seed    "$SEED"
